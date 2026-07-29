Set-StrictMode -Version Latest

$script:AllowedRemoteDevKeys = @{
    APP_ENV                    = $true
    ADDR                       = $true
    DB_TARGET                  = $true
    DB_DRIVER                  = $true
    DB_DSN                     = $true
    DB_EXPECTED_DATABASE       = $true
    DB_EXPECTED_SERVER_UUID    = $true
    DB_EXPECTED_USER           = $true
    AUTO_MIGRATE               = $true
    SEED_DEFAULTS              = $true
    FILE_UPLOAD_LOCAL_DIR      = $true
    SSH_HOST                   = $true
    SSH_LOCAL_PORT             = $true
    SSH_REMOTE_HOST            = $true
    SSH_REMOTE_PORT            = $true
}

function Get-RemoteDevRepositoryRoot {
    [CmdletBinding()]
    param()

    return [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
}

function Resolve-RemoteDevConfigPath {
    [CmdletBinding()]
    param([string]$Path)

    if ([string]::IsNullOrWhiteSpace($Path)) {
        return Join-Path (Get-RemoteDevRepositoryRoot) 'backend\.env.remote-dev'
    }
    if ([System.IO.Path]::IsPathRooted($Path)) {
        return [System.IO.Path]::GetFullPath($Path)
    }
    return [System.IO.Path]::GetFullPath((Join-Path (Get-RemoteDevRepositoryRoot) $Path))
}

function Get-RemoteDevRuntimeDirectory {
    [CmdletBinding()]
    param([switch]$Ensure)

    $path = Join-Path $PSScriptRoot '.runtime'
    if ($Ensure) {
        New-Item -ItemType Directory -Path $path -Force | Out-Null
    }
    return $path
}

function Import-RemoteDevEnvironment {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Path)

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw 'remote development configuration file was not found'
    }

    $values = @{}
    $lineNumber = 0
    foreach ($line in [System.IO.File]::ReadAllLines([System.IO.Path]::GetFullPath($Path))) {
        $lineNumber++
        $trimmedLine = $line.Trim()
        if ($trimmedLine.Length -eq 0 -or $trimmedLine.StartsWith('#')) {
            continue
        }

        $equalsIndex = $line.IndexOf('=')
        if ($equalsIndex -le 0) {
            throw "invalid dotenv syntax at line $lineNumber"
        }

        $key = $line.Substring(0, $equalsIndex).Trim()
        if ($key -notmatch '^[A-Za-z_][A-Za-z0-9_]*$') {
            throw "invalid dotenv key at line $lineNumber"
        }
        if (-not $script:AllowedRemoteDevKeys.ContainsKey($key)) {
            throw "unknown dotenv key $key at line $lineNumber"
        }
        if ($values.ContainsKey($key)) {
            throw "duplicate dotenv key $key at line $lineNumber"
        }

        $rawValue = $line.Substring($equalsIndex + 1).Trim()
        $value = $rawValue
        if ($rawValue.Length -gt 0 -and ($rawValue[0] -eq "'" -or $rawValue[0] -eq '"')) {
            $quote = $rawValue[0]
            if ($rawValue.Length -lt 2 -or $rawValue[$rawValue.Length - 1] -ne $quote) {
                throw "unpaired dotenv quote at line $lineNumber"
            }
            $value = $rawValue.Substring(1, $rawValue.Length - 2)
        }
        elseif ($rawValue.EndsWith("'") -or $rawValue.EndsWith('"')) {
            throw "unpaired dotenv quote at line $lineNumber"
        }

        $values[$key] = $value
    }

    foreach ($key in $values.Keys) {
        [Environment]::SetEnvironmentVariable($key, [string]$values[$key], 'Process')
    }
}

function Get-RequiredRemoteDevEnvironmentValue {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Name)

    $value = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if ([string]::IsNullOrWhiteSpace($value)) {
        throw "$Name is required"
    }
    return $value
}

function ConvertTo-RemoteDevPort {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][string]$Name)

    $rawValue = Get-RequiredRemoteDevEnvironmentValue -Name $Name
    $port = 0
    if (-not [int]::TryParse($rawValue, [ref]$port) -or $port -lt 1 -or $port -gt 65535) {
        throw "$Name must be a valid TCP port"
    }
    return $port
}

function Test-RemoteDevLocalPortAvailable {
    [CmdletBinding()]
    param([Parameter(Mandatory = $true)][int]$Port)

    $listener = $null
    try {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
        $listener.Start()
        return $true
    }
    catch {
        return $false
    }
    finally {
        if ($null -ne $listener) {
            $listener.Stop()
        }
    }
}

function Test-RemoteDevLocalPortOpen {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][int]$Port,
        [int]$TimeoutMilliseconds = 250
    )

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $connect = $client.BeginConnect([System.Net.IPAddress]::Loopback, $Port, $null, $null)
        if (-not $connect.AsyncWaitHandle.WaitOne($TimeoutMilliseconds)) {
            return $false
        }
        $client.EndConnect($connect)
        return $true
    }
    catch {
        return $false
    }
    finally {
        $client.Dispose()
    }
}

function Wait-RemoteDevLocalPort {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][int]$Port,
        [int]$TimeoutSeconds = 10
    )

    $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
    while ($stopwatch.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
        if (Test-RemoteDevLocalPortOpen -Port $Port) {
            return $true
        }
        Start-Sleep -Milliseconds 200
    }
    return $false
}

Export-ModuleMember -Function @(
    'ConvertTo-RemoteDevPort',
    'Get-RemoteDevRepositoryRoot',
    'Get-RemoteDevRuntimeDirectory',
    'Get-RequiredRemoteDevEnvironmentValue',
    'Import-RemoteDevEnvironment',
    'Resolve-RemoteDevConfigPath',
    'Test-RemoteDevLocalPortAvailable',
    'Test-RemoteDevLocalPortOpen',
    'Wait-RemoteDevLocalPort'
)
