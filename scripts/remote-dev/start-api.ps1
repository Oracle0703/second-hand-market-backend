[CmdletBinding()]
param([string]$ConfigPath)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

Import-Module (Join-Path $PSScriptRoot 'RemoteDev.psm1') -Force
$resolvedConfigPath = Resolve-RemoteDevConfigPath -Path $ConfigPath
Import-RemoteDevEnvironment -Path $resolvedConfigPath

$backendPath = Join-Path (Get-RemoteDevRepositoryRoot) 'backend'
Push-Location $backendPath
try {
    & go run ./scripts/verify_database
    if ($LASTEXITCODE -ne 0) {
        throw 'database identity verification failed; API was not started'
    }
    & go run ./cmd/server
    if ($LASTEXITCODE -ne 0) {
        throw 'local API exited with an error'
    }
}
finally {
    Pop-Location
}
