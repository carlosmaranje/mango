#Requires -Version 5.1
<#
.SYNOPSIS
    Uninstalls the Mango Agent Gateway from Windows.
#>

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$installer  = Join-Path $scriptDir "install.ps1"

if (Test-Path $installer) {
    & $installer -Uninstall
} else {
    Write-Error "install.ps1 not found in $scriptDir. Cannot proceed."
}
