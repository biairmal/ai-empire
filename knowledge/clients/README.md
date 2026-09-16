# Clients

One folder per client, named by its slug (`knowledge/clients/<slug>/`). Everything here is loaded for **every project of that client, and for no other project** (spec §11B).

Good content: coding conventions, branding/UI rules, compliance rules, infrastructure names (never secrets), domain vocabulary.

Projects stay in [../projects/](../projects/). A project records its client in the platform database, so moving a project to another client never moves files.
