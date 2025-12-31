$src = "..\nlm_upstream"
$dest = "."
Copy-Item -Path "$src\gen" -Destination "$dest" -Recurse -Force
Copy-Item -Path "$src\proto" -Destination "$dest" -Recurse -Force
Copy-Item -Path "$src\internal\api" -Destination "$dest\internal" -Recurse -Force
Copy-Item -Path "$src\internal\batchexecute" -Destination "$dest\internal" -Recurse -Force
Copy-Item -Path "$src\internal\rpc" -Destination "$dest\internal" -Recurse -Force
Copy-Item -Path "$src\internal\httprr" -Destination "$dest\internal" -Recurse -Force
Copy-Item -Path "$src\internal\beprotojson" -Destination "$dest\internal" -Recurse -Force
Write-Host "Copy complete"
