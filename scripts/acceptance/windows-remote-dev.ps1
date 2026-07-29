[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$remoteDevRoot = Join-Path $repoRoot 'scripts\remote-dev'
$modulePath = Join-Path $remoteDevRoot 'RemoteDev.psm1'
$failures = [System.Collections.Generic.List[string]]::new()
$moduleLoaded = $false

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) {
        throw $Message
    }
}

function Assert-Equal {
    param($Actual, $Expected, [string]$Message)
    if ($Actual -ne $Expected) {
        throw $Message
    }
}

function Invoke-Check {
    param([string]$Name, [scriptblock]$Check)
    try {
        & $Check
        Write-Output "PASS $Name"
    }
    catch {
        $failures.Add($Name)
        Write-Output "FAIL $Name"
    }
}

Invoke-Check 'module exists and imports' {
    Assert-True (Test-Path -LiteralPath $modulePath -PathType Leaf) 'RemoteDev.psm1 is missing'
    Import-Module $modulePath -Force -ErrorAction Stop
    $script:moduleLoaded = $true
}

$testRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('second-hand-remote-dev-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
try {
    Invoke-Check 'dotenv parser accepts supported values and first equals' {
        Assert-True $script:moduleLoaded 'module unavailable'
        $path = Join-Path $testRoot 'valid.env'
        @(
            '# comment',
            'APP_ENV=development',
            "ADDR='127.0.0.1:8080'",
            'DB_DSN="account:password@tcp(127.0.0.1:13307)/db?parseTime=true&loc=Local"'
        ) | Set-Content -LiteralPath $path -Encoding UTF8
        Import-RemoteDevEnvironment -Path $path
        Assert-Equal ([Environment]::GetEnvironmentVariable('APP_ENV', 'Process')) 'development' 'unquoted value mismatch'
        Assert-Equal ([Environment]::GetEnvironmentVariable('ADDR', 'Process')) '127.0.0.1:8080' 'single-quoted value mismatch'
        Assert-Equal ([Environment]::GetEnvironmentVariable('DB_DSN', 'Process')) 'account:password@tcp(127.0.0.1:13307)/db?parseTime=true&loc=Local' 'first equals parsing mismatch'
    }

    Invoke-Check 'dotenv parser rejects duplicate keys' {
        Assert-True $script:moduleLoaded 'module unavailable'
        $path = Join-Path $testRoot 'duplicate.env'
        @('APP_ENV=development', 'APP_ENV=test') | Set-Content -LiteralPath $path -Encoding UTF8
        $rejected = $false
        try { Import-RemoteDevEnvironment -Path $path } catch { $rejected = $true }
        Assert-True $rejected 'duplicate key was accepted'
    }

    Invoke-Check 'dotenv parser rejects unknown keys' {
        Assert-True $script:moduleLoaded 'module unavailable'
        $path = Join-Path $testRoot 'unknown.env'
        'UNEXPECTED_KEY=value' | Set-Content -LiteralPath $path -Encoding UTF8
        $rejected = $false
        try { Import-RemoteDevEnvironment -Path $path } catch { $rejected = $true }
        Assert-True $rejected 'unknown key was accepted'
    }

    Invoke-Check 'dotenv parser rejects invalid lines and unmatched quotes' {
        Assert-True $script:moduleLoaded 'module unavailable'
        foreach ($content in @('not-an-assignment', "DB_DSN='unterminated")) {
            $path = Join-Path $testRoot ([guid]::NewGuid().ToString('N') + '.env')
            $content | Set-Content -LiteralPath $path -Encoding UTF8
            $rejected = $false
            try { Import-RemoteDevEnvironment -Path $path } catch { $rejected = $true }
            Assert-True $rejected 'invalid dotenv line was accepted'
        }
    }

    Invoke-Check 'dotenv errors do not leak values' {
        Assert-True $script:moduleLoaded 'module unavailable'
        $sentinel = 'dotenv-secret-sentinel'
        $path = Join-Path $testRoot 'secret.env'
        ("DB_DSN='" + $sentinel) | Set-Content -LiteralPath $path -Encoding UTF8
        $message = ''
        try { Import-RemoteDevEnvironment -Path $path } catch { $message = $_.Exception.Message }
        Assert-True ($message.Length -gt 0) 'invalid secret value was accepted'
        Assert-True (-not $message.Contains($sentinel)) 'dotenv error leaked a value'
    }
}
finally {
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
    $resolvedTestRoot = [System.IO.Path]::GetFullPath($testRoot)
    if ($resolvedTestRoot.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
    }
}

Invoke-Check 'gitignore is narrowly scoped' {
    $gitignore = Get-Content -LiteralPath (Join-Path $repoRoot '.gitignore')
    Assert-True ($gitignore -contains '/backend/.env.remote-dev') 'local remote-dev env is not ignored exactly'
    Assert-True ($gitignore -contains '/scripts/remote-dev/.runtime/') 'remote-dev runtime directory is not ignored exactly'

    & git -C $repoRoot check-ignore -q 'backend/.env.remote-dev'
    Assert-Equal $LASTEXITCODE 0 'local env is not ignored'
    & git -C $repoRoot check-ignore -q 'scripts/remote-dev/.runtime/tunnel.pid'
    Assert-Equal $LASTEXITCODE 0 'runtime PID path is not ignored'
    & git -C $repoRoot check-ignore -q 'backend/configs/.env.remote-dev.example'
    Assert-True ($LASTEXITCODE -ne 0) 'tracked template is ignored'
}

Invoke-Check 'configuration template contains only safe placeholders' {
    $templatePath = Join-Path $repoRoot 'backend\configs\.env.remote-dev.example'
    Assert-True (Test-Path -LiteralPath $templatePath -PathType Leaf) 'remote-dev template is missing'
    $template = Get-Content -LiteralPath $templatePath
    foreach ($line in @(
        'APP_ENV=development',
        'ADDR=127.0.0.1:8080',
        'DB_TARGET=remote-development',
        'DB_DRIVER=mysql',
        'DB_EXPECTED_DATABASE=second_hand_market_dev',
        'DB_EXPECTED_USER=shm_dev_app',
        'AUTO_MIGRATE=false',
        'SEED_DEFAULTS=false',
        'FILE_UPLOAD_LOCAL_DIR=runtime/dev-uploads',
        'SSH_HOST=yu',
        'SSH_LOCAL_PORT=13307',
        'SSH_REMOTE_HOST=127.0.0.1',
        'SSH_REMOTE_PORT=3307'
    )) {
        Assert-True ($template -contains $line) "template is missing $line"
    }
    $dsnLine = $template | Where-Object { $_ -like 'DB_DSN=*' }
    $uuidLine = $template | Where-Object { $_ -like 'DB_EXPECTED_SERVER_UUID=*' }
    Assert-Equal @($dsnLine).Count 1 'template must contain one DB_DSN'
    Assert-Equal @($uuidLine).Count 1 'template must contain one expected UUID'
    Assert-True ($dsnLine -match 'replace-with') 'DB_DSN must remain a placeholder'
    Assert-True ($uuidLine -match 'replace-with') 'server UUID must remain a placeholder'
}

Invoke-Check 'PowerShell tools parse and retain safe SSH policy' {
    $paths = @(
        (Join-Path $remoteDevRoot 'start-tunnel.ps1'),
        (Join-Path $remoteDevRoot 'stop-tunnel.ps1'),
        (Join-Path $remoteDevRoot 'test-database.ps1'),
        (Join-Path $remoteDevRoot 'start-api.ps1')
    )
    foreach ($path in $paths) {
        Assert-True (Test-Path -LiteralPath $path -PathType Leaf) "missing script $path"
        $tokens = $null
        $parseErrors = $null
        [System.Management.Automation.Language.Parser]::ParseFile($path, [ref]$tokens, [ref]$parseErrors) | Out-Null
        Assert-Equal @($parseErrors).Count 0 "PowerShell parse failure in $path"
    }
    $allText = (($paths + $modulePath) | ForEach-Object { Get-Content -LiteralPath $_ -Raw }) -join "`n"
    Assert-True ($allText -notmatch 'StrictHostKeyChecking\s*=\s*no') 'unsafe host key bypass is present'
    $startTunnel = Get-Content -LiteralPath (Join-Path $remoteDevRoot 'start-tunnel.ps1') -Raw
    foreach ($required in @('ExitOnForwardFailure=yes', 'ServerAliveInterval=30', 'ServerAliveCountMax=3', "'-T'", "'-N'", '127.0.0.1')) {
        Assert-True ($startTunnel.Contains($required)) "start-tunnel is missing $required"
    }
    $stopTunnel = Get-Content -LiteralPath (Join-Path $remoteDevRoot 'stop-tunnel.ps1') -Raw
    Assert-True ($stopTunnel.Contains('Get-CimInstance')) 'stop-tunnel does not inspect the target process'
    Assert-True ($stopTunnel.Contains('CommandLine')) 'stop-tunnel does not validate the command line'
    Assert-True ($stopTunnel.Contains('Stop-Process')) 'stop-tunnel does not stop a validated PID'
}

Invoke-Check 'Go verify command reuses server identity startup without writes' {
    $mainPath = Join-Path $repoRoot 'backend\scripts\verify_database\main.go'
    Assert-True (Test-Path -LiteralPath $mainPath -PathType Leaf) 'verify_database command is missing'
    $mainText = Get-Content -LiteralPath $mainPath -Raw
    Assert-True ($mainText.Contains('app.LoadConfig')) 'verify command does not load app.Config'
    Assert-True ($mainText.Contains('app.NewServer')) 'verify command does not reuse server identity startup'
    Assert-True (-not $mainText.Contains('MigrateSchema')) 'verify command must not migrate'
    Assert-True (-not $mainText.Contains('SeedDefaultCategories')) 'verify command must not seed'
    $goRuntime = Join-Path $remoteDevRoot '.runtime\acceptance-go'
    $goCache = Join-Path $goRuntime 'cache'
    $goTemp = Join-Path $goRuntime ('tmp-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $goCache -Force | Out-Null
    New-Item -ItemType Directory -Path $goTemp -Force | Out-Null
    $previousGoCache = [Environment]::GetEnvironmentVariable('GOCACHE', 'Process')
    $previousGoTemp = [Environment]::GetEnvironmentVariable('GOTMPDIR', 'Process')
    [Environment]::SetEnvironmentVariable('GOCACHE', $goCache, 'Process')
    [Environment]::SetEnvironmentVariable('GOTMPDIR', $goTemp, 'Process')
    Push-Location (Join-Path $repoRoot 'backend')
    try {
        & go test -p 1 -count=1 ./scripts/verify_database
        Assert-Equal $LASTEXITCODE 0 'verify_database Go tests failed'
    }
    finally {
        Pop-Location
        [Environment]::SetEnvironmentVariable('GOCACHE', $previousGoCache, 'Process')
        [Environment]::SetEnvironmentVariable('GOTMPDIR', $previousGoTemp, 'Process')
    }
}

if ($failures.Count -gt 0) {
    Write-Output "WINDOWS_REMOTE_DEV FAIL ($($failures.Count))"
    exit 1
}

Write-Output 'WINDOWS_REMOTE_DEV PASS'
