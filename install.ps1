# autopus-adk Windows install script
# Usage: irm https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.ps1 | iex
param([switch]$LibraryOnly)
$ErrorActionPreference = "Stop"
$Repo = "autopus-ai/autopus-adk"
$Binary = "auto.exe"
$AliasBinary = "autopus.exe"
$SigningFloor = "0.50.73"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\autopus-adk\bin" }
$P256SpkiPrefixHex = "3059301306072a8648ce3d020106082a8648ce3d03010703420004"
$Ecs1HeaderHex = "4543533120000000"
$P256OrderHex = "ffffffff00000000ffffffffffffffffbce6faada7179e84f3b9cac2fc632551"
$ReleaseSigningKeys = @(
    [pscustomobject]@{ Fingerprint = "e1fdfe066484c7eae8ff16fa4b1ee6237b8d06299c2b66ced485f029af77837f"; ExpiresAt = "2028-07-17"; SpkiBase64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEDFjY80Lc2GJSsd8M6uAO/v7AZK3Z1sPEXrK4Hbm4m4+ykavvcoKlpZ5sn/T/l2InDXuhxkdX6aFv57bicik2Ug==" },
    [pscustomobject]@{ Fingerprint = "93d9f681d829f2d0bdba7e1853e6acf9ae2ffd2c760355853218e920c35cc5ff"; ExpiresAt = "2030-07-17"; SpkiBase64 = "MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEp+d1byDqWFismSIMWhTEHnbo/pdp7JVZwhXOIZJb0q2WHLxwMD7P77Fkr75Xnx1qYZgfvIl9Sg8Z+V9gSaq8Og==" }
)
function Info($msg) { Write-Host $msg -ForegroundColor Cyan }
function Ok($msg) { Write-Host $msg -ForegroundColor Green }
function Err($msg) { Write-Host $msg -ForegroundColor Red; exit 1 }
function Convert-HexBytes([string]$Hex) {
    if (($Hex.Length % 2) -ne 0 -or $Hex -cnotmatch '\A[0-9a-f]+\z') { throw "invalid hex" }
    $bytes = New-Object byte[] ($Hex.Length / 2)
    for ($i = 0; $i -lt $bytes.Length; $i++) { $bytes[$i] = [Convert]::ToByte($Hex.Substring($i * 2, 2), 16) }
    return ,$bytes
}
function Get-Sha256Hex([byte[]]$Data) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $digest = $sha.ComputeHash($Data) } finally { $sha.Dispose() }
    return -join ($digest | ForEach-Object { $_.ToString("x2") })
}
function Convert-SpkiToEcs1([byte[]]$Spki) {
    $prefix = Convert-HexBytes $P256SpkiPrefixHex
    if ($Spki.Length -ne 91) { throw "malformed P-256 SPKI: expected 91 bytes" }
    for ($i = 0; $i -lt $prefix.Length; $i++) {
        if ($Spki[$i] -ne $prefix[$i]) { throw "malformed P-256 SPKI: unexpected prefix" }
    }
    $blob = New-Object byte[] 72
    $header = Convert-HexBytes $Ecs1HeaderHex
    [Array]::Copy($header, 0, $blob, 0, 8)
    [Array]::Copy($Spki, 27, $blob, 8, 64)
    return ,$blob
}
function Convert-DerScalar([byte[]]$Der, [ref]$Offset) {
    $start = $Offset.Value
    if ($start + 2 -gt $Der.Length -or $Der[$start] -ne 0x02) { throw "canonical P-256 DER signature required" }
    $encodedLength = [int]$Der[$start + 1]
    if ($encodedLength -lt 1 -or $encodedLength -gt 33 -or $start + 2 + $encodedLength -gt $Der.Length) {
        throw "canonical P-256 DER signature required"
    }
    $valueStart = $start + 2
    $valueLength = $encodedLength
    if ($Der[$valueStart] -eq 0) {
        if ($valueLength -eq 1 -or $Der[$valueStart + 1] -lt 0x80) { throw "canonical P-256 DER signature required" }
        $valueStart++
        $valueLength--
    } elseif ($Der[$valueStart] -ge 0x80) { throw "canonical P-256 DER signature required" }
    if ($valueLength -gt 32) { throw "canonical P-256 DER signature required" }
    $scalar = New-Object byte[] 32
    [Array]::Copy($Der, $valueStart, $scalar, 32 - $valueLength, $valueLength)
    $order = Convert-HexBytes $P256OrderHex
    $less = $false
    for ($i = 0; $i -lt 32; $i++) {
        if ($scalar[$i] -lt $order[$i]) { $less = $true; break }
        if ($scalar[$i] -gt $order[$i]) { throw "canonical P-256 DER signature required" }
    }
    if (-not $less) { throw "canonical P-256 DER signature required" }
    $Offset.Value = $start + 2 + $encodedLength
    return ,$scalar
}
function Convert-DerSignatureToP1363([byte[]]$Der) {
    if ($Der.Length -lt 8 -or $Der.Length -gt 72 -or $Der[0] -ne 0x30 -or $Der[1] -ne $Der.Length - 2) {
        throw "canonical P-256 DER signature required"
    }
    $offset = 2
    $r = Convert-DerScalar $Der ([ref]$offset)
    $s = Convert-DerScalar $Der ([ref]$offset)
    if ($offset -ne $Der.Length) { throw "canonical P-256 DER signature required" }
    $signature = New-Object byte[] 64
    [Array]::Copy($r, 0, $signature, 0, 32)
    [Array]::Copy($s, 0, $signature, 32, 32)
    return ,$signature
}
function Read-ReleaseSignatureEnvelope([byte[]]$Envelope) {
    if ($null -eq $Envelope -or $Envelope.Length -lt 1 -or $Envelope.Length -gt 4096) { throw "malformed release signature envelope: size" }
    foreach ($value in $Envelope) {
        if (($value -lt 0x20 -and $value -ne 9 -and $value -ne 10) -or $value -gt 0x7e) {
            throw "malformed release signature envelope: non-ASCII or forbidden control byte"
        }
    }
    if ($Envelope[$Envelope.Length - 1] -ne 10) { throw "malformed release signature envelope: final LF required" }
    $lines = [Text.Encoding]::ASCII.GetString($Envelope).Split([char]10)
    if ($lines.Count -lt 3 -or $lines[0] -cne "AUTOPUS-RELEASE-SIGNATURE-V1" -or $lines[$lines.Count - 1] -cne "") {
        throw "malformed release signature envelope: header or layout"
    }
    $count = $lines.Count - 2
    if ($count -lt 1 -or $count -gt 16) { throw "malformed release signature envelope: record count" }
    $seen = @{}
    $records = @()
    for ($i = 1; $i -le $count; $i++) {
        $line = $lines[$i]
        if ($line.Length -lt 1 -or $line.Length -gt 256 -or $line -cnotmatch '\A([0-9a-f]{64})\t([A-Za-z0-9+/]+={0,2})\z') {
            throw "malformed release signature envelope: record shape"
        }
        $fingerprint = $Matches[1]
        $encoded = $Matches[2]
        if ($seen.ContainsKey($fingerprint)) { throw "malformed release signature envelope: duplicate" }
        $seen[$fingerprint] = $true
        try { $der = [Convert]::FromBase64String($encoded) } catch { throw "malformed release signature envelope: base64" }
        if ([Convert]::ToBase64String($der) -cne $encoded) { throw "malformed release signature envelope: non-canonical base64" }
        try { $p1363 = Convert-DerSignatureToP1363 $der } catch { throw "malformed release signature envelope: $($_.Exception.Message)" }
        $records += [pscustomobject]@{ Fingerprint = $fingerprint; Signature = $p1363 }
    }
    return ,$records
}
function Test-ReleaseSignature([byte[]]$Checksums, [byte[]]$Envelope, [object[]]$Keys = $ReleaseSigningKeys, [DateTime]$Now = [DateTime]::UtcNow) {
    $records = Read-ReleaseSignatureEnvelope $Envelope
    if ($null -eq $Keys -or @($Keys).Count -eq 0) { throw "malformed embedded release signing key: no keys configured" }
    $seen = @{}
    $active = @{}
    $today = $Now.ToUniversalTime().ToString("yyyy-MM-dd", [Globalization.CultureInfo]::InvariantCulture)
    foreach ($key in @($Keys)) {
        $fingerprint = [string]$key.Fingerprint
        if ($fingerprint -cnotmatch '\A[0-9a-f]{64}\z' -or $seen.ContainsKey($fingerprint)) { throw "malformed embedded release signing key: fingerprint" }
        $seen[$fingerprint] = $true
        try { $spki = [Convert]::FromBase64String([string]$key.SpkiBase64) } catch { throw "malformed embedded release signing key: base64" }
        if ([Convert]::ToBase64String($spki) -cne [string]$key.SpkiBase64 -or
            (Get-Sha256Hex $spki) -cne $fingerprint) {
            throw "malformed embedded release signing key: SPKI fingerprint"
        }
        $cng = $null
        try {
            $blob = Convert-SpkiToEcs1 $spki
            $cng = [System.Security.Cryptography.CngKey]::Import(
                $blob, [System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
        } catch {
            throw "malformed embedded release signing key: $($_.Exception.Message)"
        } finally {
            if ($null -ne $cng) { $cng.Dispose() }
        }
        $expiry = [DateTime]::MinValue
        if (-not [DateTime]::TryParseExact([string]$key.ExpiresAt, "yyyy-MM-dd", [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::None, [ref]$expiry)) {
            throw "malformed embedded release signing key: expiry"
        }
        if ([String]::CompareOrdinal($today, [string]$key.ExpiresAt) -le 0) { $active[$fingerprint] = $blob }
    }
    if ($active.Count -eq 0) { throw "all embedded release signing keys expired" }
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $digest = $sha.ComputeHash($Checksums) } finally { $sha.Dispose() }
    foreach ($record in $records) {
        if (-not $active.ContainsKey($record.Fingerprint)) { continue }
        $cng = $null
        $ecdsa = $null
        try {
            $cng = [System.Security.Cryptography.CngKey]::Import(
                $active[$record.Fingerprint], [System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
            $ecdsa = New-Object System.Security.Cryptography.ECDsaCng -ArgumentList $cng
            if ($ecdsa.VerifyHash($digest, [byte[]]$record.Signature)) { return $true }
        } finally {
            if ($null -ne $ecdsa) { $ecdsa.Dispose() }
            if ($null -ne $cng) { $cng.Dispose() }
        }
    }
    throw "no trusted release signing key verified"
}
function Test-PathContainsDir([string]$PathValue, [string]$Dir) {
    if (-not $PathValue) { return $false }
    $needle = $Dir.TrimEnd('\').ToLowerInvariant()
    foreach ($entry in ($PathValue -split ';')) {
        if ($entry.TrimEnd('\').ToLowerInvariant() -eq $needle) { return $true }
    }
    return $false
}
function Get-GitBashPath([string]$Dir) {
    if ($Dir -match '^([A-Za-z]):\\(.*)$') {
        return "/$($matches[1].ToLowerInvariant())/$(($matches[2] -replace '\\', '/'))"
    }
    return $Dir -replace '\\', '/'
}
function Show-PathHint([string]$InstallDir, [bool]$PathAdded) {
    Ok "  Installed commands:"
    Ok "    auto"
    Ok "    autopus    # auto alias"
    if (Test-PathContainsDir $env:Path $InstallDir) {
        Ok "  PATH ready in this PowerShell session: $InstallDir"
    } else {
        Write-Host "  PATH was updated for future shells, but this parent shell may need a restart." -ForegroundColor Yellow
    }
    if ($PathAdded) { Write-Host "  New terminals will pick up: $InstallDir" -ForegroundColor Yellow }
    if ($env:MSYSTEM) {
        $bashPath = Get-GitBashPath $InstallDir
        Write-Host "  If you installed from Git Bash, reopen this window or run:" -ForegroundColor Yellow
        Write-Host ('    export PATH="{0}:$PATH"' -f $bashPath) -ForegroundColor Yellow
    }
}
function Show-NextSteps {
    @(
        "  Next steps:", "    auto init", "      Initialize the current project.",
        "      Detect installed AI coding CLIs and generate autopus.yaml plus platform harness files.", "",
        "    auto update --self", "      Update the auto CLI binary itself to the latest release.", "",
        "    auto update", "      Refresh rules, skills, agents, and generated harness files in the current project.", "",
        "  Recommended order:", "    1. New project: auto init", "    2. New CLI release: auto update --self",
        "    3. Then inside your project: auto update"
    ) | ForEach-Object { Ok $_ }
}
function Get-Arch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Err "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}
function Get-LatestVersion {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "autopus-installer" }
    return $release.tag_name -replace '^v', ''
}
function Verify-Checksum($file, $expected) {
    $actual = (Get-FileHash -Path $file -Algorithm SHA256).Hash.ToLower()
    if ($actual -ne $expected) { Err "Checksum mismatch!`n  expected: $expected`n  actual:   $actual" }
}
function Add-InstallerPath([string]$Dir) {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathAdded = $false
    if (-not (Test-PathContainsDir $userPath $Dir)) {
        $newUserPath = if ($userPath) { "$userPath;$Dir" } else { $Dir }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
        Info "Added $Dir to user PATH"; $pathAdded = $true
    }
    if (-not (Test-PathContainsDir $env:Path $Dir)) { $env:Path = if ($env:Path) { "$env:Path;$Dir" } else { $Dir } }
    return $pathAdded
}
function Main {
    $Arch = Get-Arch
    $Version = if ($env:VERSION) { $env:VERSION } else { Get-LatestVersion }
    if (-not $Version) { Err "Failed to get latest version. Check GitHub API limits." }
    if ($Version -cnotmatch '\A[0-9]+\.[0-9]+\.[0-9]+\z' -or [Version]$Version -lt [Version]$SigningFloor) { Err "unsigned_release_not_supported: v$SigningFloor or newer is required" }
    Info "autopus-adk v$Version installing... (windows/$Arch)"
    $Archive = "autopus-adk_${Version}_windows_${Arch}.zip"
    $BaseUrl = "https://github.com/$Repo/releases/download/v$Version"
    $Url = "$BaseUrl/$Archive"
    $ChecksumsUrl = "$BaseUrl/checksums.txt"
    $SignaturesUrl = "$BaseUrl/checksums.txt.signatures"
    $TmpDir = Join-Path $env:TEMP "autopus-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null
    try {
        Info "Verifying release signature..."
        Invoke-WebRequest -Uri $ChecksumsUrl -OutFile "$TmpDir\checksums.txt" -UseBasicParsing
        Invoke-WebRequest -Uri $SignaturesUrl -OutFile "$TmpDir\checksums.txt.signatures" -UseBasicParsing
        $checksumBytes = [IO.File]::ReadAllBytes("$TmpDir\checksums.txt")
        $envelopeBytes = [IO.File]::ReadAllBytes("$TmpDir\checksums.txt.signatures")
        try { [void](Test-ReleaseSignature $checksumBytes $envelopeBytes) } catch {
            Err "Release signature verification failed: $($_.Exception.Message)"
        }
        Ok "Release signature verified"
        Info "Downloading: $Url"
        Invoke-WebRequest -Uri $Url -OutFile "$TmpDir\$Archive" -UseBasicParsing
        Info "Verifying checksum..."
        $pattern = "^([0-9a-f]{64})  " + [Regex]::Escape($Archive) + "$"
        $checksumLines = @([Text.Encoding]::ASCII.GetString($checksumBytes).Split([char]10) |
            Where-Object { $_ -cmatch $pattern })
        if ($checksumLines.Count -ne 1) { Err "Checksum not found for $Archive in checksums.txt" }
        $expected = ($checksumLines[0] -split '  ', 2)[0]
        Verify-Checksum "$TmpDir\$Archive" $expected
        Ok "Checksum verified"
        Info "Extracting..."
        Expand-Archive -Path "$TmpDir\$Archive" -DestinationPath $TmpDir -Force
        if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }
        Info "Installing to $InstallDir\$Binary..."
        $TargetPath = "$InstallDir\$Binary"
        $OldPath = "$TargetPath.old"
        if (Test-Path $TargetPath) {
            Remove-Item $OldPath -Force -ErrorAction SilentlyContinue
            try { Rename-Item $TargetPath $OldPath -Force } catch {}
        }
        Copy-Item "$TmpDir\auto.exe" $TargetPath -Force
        Copy-Item "$TmpDir\auto.exe" "$InstallDir\$AliasBinary" -Force
        Remove-Item $OldPath -Force -ErrorAction SilentlyContinue
        $pathAdded = Add-InstallerPath $InstallDir
        Ok "autopus-adk v$Version installed!"
        Ok ""
        Show-PathHint $InstallDir $pathAdded
        Ok ""
        Info "Checking required tools... (skips anything already installed)"
        try {
            & "$InstallDir\$Binary" doctor --fix --yes --required-only 2>$null
            Ok "Required tools checked!"
        } catch {
            Write-Host "  Some required tools could not be auto-installed." -ForegroundColor Yellow
            Write-Host "  Run manually: auto doctor" -ForegroundColor Yellow
        }
        Ok ""
        Ok "$([char]::ConvertFromUtf32(0x1F419)) Autopus-ADK is ready!"
        Ok ""
        Show-NextSteps
    } finally {
        Remove-Item -Path $TmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
if (-not $LibraryOnly) { Main }
