$path = "."
$files = Get-ChildItem -Path $path -Recurse -Include *.go
foreach ($file in $files) {
    $content = Get-Content $file.FullName -Raw -Encoding UTF8
    if ($content -match "github.com/tmc/nlm") {
        $newContent = $content.Replace("github.com/tmc/nlm", "notebook-podcast-automator")
        Set-Content -Path $file.FullName -Value $newContent -NoNewline -Encoding UTF8
        Write-Host "Updated: $($file.Name)"
    }
}
Write-Host "Import fix complete"
