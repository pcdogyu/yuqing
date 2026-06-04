$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$xPath = Join-Path $root "internal\provider\cryptosocial\testdata\x_feed.json"
$tgPath = Join-Path $root "internal\provider\cryptosocial\testdata\telegram_feed.json"

$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add("http://127.0.0.1:19090/")
$listener.Start()

Write-Host "Mock crypto social server listening on http://127.0.0.1:19090/"
Write-Host "X endpoint:        http://127.0.0.1:19090/mock/x"
Write-Host "Telegram endpoint: http://127.0.0.1:19090/mock/telegram"
Write-Host "Press Ctrl+C to stop."

try {
    while ($listener.IsListening) {
        $context = $listener.GetContext()
        $request = $context.Request
        $response = $context.Response

        $payloadPath = switch ($request.Url.AbsolutePath) {
            "/mock/x" { $xPath; break }
            "/mock/telegram" { $tgPath; break }
            default { $null }
        }

        if ($null -eq $payloadPath) {
            $response.StatusCode = 404
            $buffer = [System.Text.Encoding]::UTF8.GetBytes('{"code":404,"message":"not found"}')
        } else {
            $response.StatusCode = 200
            $buffer = [System.Text.Encoding]::UTF8.GetBytes((Get-Content -Raw -Path $payloadPath))
        }

        $response.ContentType = "application/json; charset=utf-8"
        $response.ContentLength64 = $buffer.Length
        $response.OutputStream.Write($buffer, 0, $buffer.Length)
        $response.OutputStream.Close()
    }
}
finally {
    $listener.Stop()
    $listener.Close()
}
