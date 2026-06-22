$script:Passed = 0
$script:Failed = 0
$script:DriftBin = ""

function Set-DriftBinary {
    param([string]$Bin)
    $script:DriftBin = $Bin
}

function New-TestDir {
    $dir = Join-Path $env:TEMP "drift-test-$(Get-Random)"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    return $dir
}

function Remove-TestDir {
    param([string]$Dir)
    if (Test-Path $Dir) {
        Remove-Item -Recurse -Force $Dir -ErrorAction SilentlyContinue
    }
}

function Invoke-Drift {
    param([string[]]$Arguments)
    $output = & $script:DriftBin @Arguments 2>&1
    $exitCode = $LASTEXITCODE
    return @{ Output = ($output -join "`n"); ExitCode = $exitCode }
}

function Assert-ExitCode {
    param([int]$Expected, [int]$Actual, [string]$TestName)
    if ($Actual -eq $Expected) {
        return $true
    }
    Write-Host "  FAIL: $TestName - Expected exit code $Expected, got $Actual" -ForegroundColor Red
    return $false
}

function Assert-OutputContains {
    param([string]$Output, [string]$Expected, [string]$TestName)
    if ($Output -match [regex]::Escape($Expected)) {
        return $true
    }
    Write-Host "  FAIL: $TestName - Output does not contain '$Expected'" -ForegroundColor Red
    Write-Host "    Got: $Output" -ForegroundColor Gray
    return $false
}

function Assert-OutputNotContains {
    param([string]$Output, [string]$Expected, [string]$TestName)
    if ($Output -notmatch [regex]::Escape($Expected)) {
        return $true
    }
    Write-Host "  FAIL: $TestName - Output should not contain '$Expected'" -ForegroundColor Red
    return $false
}

function Assert-PathExists {
    param([string]$Path, [string]$TestName)
    if (Test-Path $Path) {
        return $true
    }
    Write-Host "  FAIL: $TestName - Path does not exist: $Path" -ForegroundColor Red
    return $false
}

function Assert-PathNotExists {
    param([string]$Path, [string]$TestName)
    if (-not (Test-Path $Path)) {
        return $true
    }
    Write-Host "  FAIL: $TestName - Path should not exist: $Path" -ForegroundColor Red
    return $false
}

function Assert-FileContent {
    param([string]$Path, [string]$Expected, [string]$TestName)
    if (-not (Test-Path $Path)) {
        Write-Host "  FAIL: $TestName - File not found: $Path" -ForegroundColor Red
        return $false
    }
    $content = Get-Content $Path -Raw
    if ($content.Trim() -eq $Expected.Trim()) {
        return $true
    }
    Write-Host "  FAIL: $TestName - File content mismatch" -ForegroundColor Red
    Write-Host "    Expected: $Expected" -ForegroundColor Gray
    Write-Host "    Got: $content" -ForegroundColor Gray
    return $false
}

function Pass-Test {
    param([string]$TestName)
    $script:Passed++
    Write-Host "  PASS: $TestName" -ForegroundColor Green
}

function Fail-Test {
    param([string]$TestName, [string]$Reason = "")
    $script:Failed++
    Write-Host "  FAIL: $TestName" -ForegroundColor Red
    if ($Reason) {
        Write-Host "    $Reason" -ForegroundColor Gray
    }
}

function Get-TestResults {
    return @{ Passed = $script:Passed; Failed = $script:Failed }
}

function Reset-Counters {
    $script:Passed = 0
    $script:Failed = 0
}
