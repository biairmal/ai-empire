package controlplane

import (
	"context"
	"errors"
	"net/http"

	"aiempire/internal/api"
	"aiempire/internal/knowledge"

	"github.com/jackc/pgx/v5"
)

// Clients (spec §11B)

func (s *Server) createClient(r *http.Request, a actor) (any, error) {
	var in api.Client
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Slug == "" || in.Name == "" {
		return nil, errf(http.StatusBadRequest, "slug and name are required")
	}
	if in.DefaultAutonomyLevel == "" {
		in.DefaultAutonomyLevel = "conservative"
	}
	var out api.Client
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		var err error
		out, err = one[api.Client](r.Context(), tx,
			`INSERT INTO clients (slug, name, default_autonomy_level) VALUES ($1, $2, $3) RETURNING *`,
			in.Slug, in.Name, in.DefaultAutonomyLevel)
		if err != nil {
			return err
		}
		return audit(r.Context(), tx, a, "client.create", clientRef(out.ID), M{"client": out})
	})
	return out, err
}

func (s *Server) listClients(r *http.Request, a actor) (any, error) {
	return collect[api.Client](r.Context(), s.db, `SELECT * FROM clients ORDER BY id`)
}

func (s *Server) getClient(r *http.Request, a actor) (any, error) {
	return clientByRef(r.Context(), s.db, r.PathValue("ref"))
}

// clientByRef finds a client by slug or numeric id (slugs always contain a letter).
func clientByRef(ctx context.Context, q querier, ref string) (api.Client, error) {
	c, err := one[api.Client](ctx, q, `SELECT * FROM clients WHERE slug = $1 OR id::text = $1`, ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, errf(http.StatusBadRequest, "unknown client %q", ref)
	}
	return c, err
}

// clientIDFor turns an optional client slug into a nullable client id.
func clientIDFor(ctx context.Context, q querier, ref string) (*int64, *api.Client, error) {
	if ref == "" {
		return nil, nil, nil
	}
	c, err := clientByRef(ctx, q, ref)
	if err != nil {
		return nil, nil, err
	}
	return &c.ID, &c, nil
}

// Projects (spec §11A)

func (s *Server) createProject(r *http.Request, a actor) (any, error) {
	var in api.CreateProject
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Slug == "" || in.Name == "" {
		return nil, errf(http.StatusBadRequest, "slug and name are required")
	}
	var out api.Project
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		clientID, client, err := clientIDFor(ctx, tx, in.Client)
		if err != nil {
			return err
		}
		autonomy := in.AutonomyLevel
		if autonomy == "" {
			autonomy = "conservative"
			if client != nil {
				autonomy = client.DefaultAutonomyLevel
			}
		}
		out, err = one[api.Project](ctx, tx,
			`INSERT INTO projects (slug, name, client_id, autonomy_level) VALUES ($1, $2, $3, $4) RETURNING *`,
			in.Slug, in.Name, clientID, autonomy)
		if err != nil {
			return err
		}
		return audit(ctx, tx, a, "project.create", projectRef(out.ID), M{"project": out, "client": in.Client})
	})
	return out, err
}

func (s *Server) listProjects(r *http.Request, a actor) (any, error) {
	return collect[api.Project](r.Context(), s.db, `SELECT * FROM projects ORDER BY id`)
}

func (s *Server) getProject(r *http.Request, a actor) (any, error) {
	return projectByRef(r.Context(), s.db, r.PathValue("ref"))
}

// projectByRef finds a project by slug or numeric id (slugs always contain a letter).
func projectByRef(ctx context.Context, q querier, ref string) (api.Project, error) {
	p, err := one[api.Project](ctx, q, `SELECT * FROM projects WHERE slug = $1 OR id::text = $1`, ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, errf(http.StatusNotFound, "unknown project %q", ref)
	}
	return p, err
}

// moveProject changes a project's client (spec §11B: explicit, audited).
// Autonomy is left as is; change it deliberately if the new client needs it.
func (s *Server) moveProject(r *http.Request, a actor) (any, error) {
	var in api.MoveProject
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	var out api.Project
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		p, err := projectByRef(ctx, tx, r.PathValue("ref"))
		if err != nil {
			return err
		}
		clientID, _, err := clientIDFor(ctx, tx, in.Client)
		if err != nil {
			return err
		}
		// Dependencies must never link two clients' work (spec §11A).
		var other string
		err = tx.QueryRow(ctx, `
			SELECT op.slug FROM task_dependencies d
			JOIN tasks a ON a.id = d.task_id
			JOIN tasks b ON b.id = d.depends_on_task_id
			JOIN projects op ON op.id = CASE WHEN a.project_id = $1 THEN b.project_id ELSE a.project_id END
			WHERE a.project_id <> b.project_id AND $1 IN (a.project_id, b.project_id)
			  AND op.client_id IS DISTINCT FROM $2
			LIMIT 1`, p.ID, clientID).Scan(&other)
		if err == nil {
			return errf(http.StatusConflict, "project %s has task dependencies with project %s, which would end up under a different client", p.Slug, other)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out, err = one[api.Project](ctx, tx, `UPDATE projects SET client_id = $2 WHERE id = $1 RETURNING *`, p.ID, clientID)
		if err != nil {
			return err
		}
		return audit(ctx, tx, a, "project.move_client", projectRef(p.ID),
			M{"from_client_id": p.ClientID, "to_client_id": clientID, "to_client": in.Client})
	})
	return out, err
}

// Repositories (spec §11A)

func (s *Server) createRepository(r *http.Request, a actor) (any, error) {
	var in api.CreateRepository
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Name == "" || in.RepoURL == "" || in.Stack == "" {
		return nil, errf(http.StatusBadRequest, "name, repo_url and stack are required")
	}
	if in.DefaultBranch == "" {
		in.DefaultBranch = "main"
	}
	var out api.Repository
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		p, err := projectByRef(ctx, tx, r.PathValue("ref"))
		if err != nil {
			return err
		}
		out, err = one[api.Repository](ctx, tx, `
			INSERT INTO repositories (project_id, name, repo_url, default_branch, stack, test_command)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING *`,
			p.ID, in.Name, in.RepoURL, in.DefaultBranch, in.Stack, in.TestCommand)
		if err != nil {
			return err
		}
		return audit(ctx, tx, a, "repository.create", projectRef(p.ID), M{"repository": out})
	})
	return out, err
}

func (s *Server) listProjectRepositories(r *http.Request, a actor) (any, error) {
	p, err := projectByRef(r.Context(), s.db, r.PathValue("ref"))
	if err != nil {
		return nil, err
	}
	return collect[api.Repository](r.Context(), s.db, `SELECT * FROM repositories WHERE project_id = $1 ORDER BY id`, p.ID)
}

func (s *Server) listRepositories(r *http.Request, a actor) (any, error) {
	return collect[api.Repository](r.Context(), s.db, `SELECT * FROM repositories ORDER BY id`)
}

// scopeFor is the knowledge a task in this project+repository may see.
func scopeFor(ctx context.Context, q querier, projectID, repoID int64, docs []string) (knowledge.Scope, error) {
	sc := knowledge.Scope{Docs: docs}
	err := q.QueryRow(ctx, `
		SELECT r.stack, p.slug, coalesce(c.slug, '')
		FROM projects p
		JOIN repositories r ON r.project_id = p.id
		LEFT JOIN clients c ON c.id = p.client_id
		WHERE p.id = $1 AND r.id = $2`, projectID, repoID).Scan(&sc.Stack, &sc.Project, &sc.Client)
	return sc, err
}
