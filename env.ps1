# Load .env and put bin\ on PATH for the current PowerShell session.
# Usage (note the leading dot + space):   . .\env.ps1
$root = $PSScriptRoot
if (-not (Test-Path "$root\.env")) {
    Copy-Item "$root\.env.example" "$root\.env"
    Write-Host "Created .env from .env.example"
}
Get-Content "$root\.env" | Where-Object { $_ -match '^[^#].*=' } | ForEach-Object {
    $k, $v = $_ -split '=', 2
    Set-Item "env:$($k.Trim())" $v.Trim()
}
if ($env:Path -notlike "*$root\bin*") { $env:Path += ";$root\bin" }
if (-not (Test-Path "$root\bin\empire.exe")) {
    Write-Host "bin\empire.exe not found - run: go build -o bin/ ./cmd/..."
}
Write-Host "AI Empire env loaded. Try: empire task list"
