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
Write-Host "Non-200 endpoint:  http://127.0.0.1:19090/mock/non-200"
Write-Host "Bad JSON endpoint: http://127.0.0.1:19090/mock/bad-json"
Write-Host "Empty endpoint:    http://127.0.0.1:19090/mock/empty"
Write-Host "Duplicate endpoint:http://127.0.0.1:19090/mock/duplicate"
Write-Host "Press Ctrl+C to stop."

try {
    while ($listener.IsListening) {
        $context = $listener.GetContext()
        $request = $context.Request
        $response = $context.Response

        switch ($request.Url.AbsolutePath) {
            "/mock/x" {
                $response.StatusCode = 200
                $buffer = [System.Text.Encoding]::UTF8.GetBytes((Get-Content -Raw -Path $xPath))
                break
            }
            "/mock/telegram" {
                $response.StatusCode = 200
                $buffer = [System.Text.Encoding]::UTF8.GetBytes((Get-Content -Raw -Path $tgPath))
                break
            }
            "/mock/non-200" {
                $response.StatusCode = 502
                $buffer = [System.Text.Encoding]::UTF8.GetBytes('{"code":502,"message":"mock upstream failed"}')
                break
            }
            "/mock/bad-json" {
                $response.StatusCode = 200
                $buffer = [System.Text.Encoding]::UTF8.GetBytes('{"items": [')
                break
            }
            "/mock/empty" {
                $response.StatusCode = 200
                $buffer = [System.Text.Encoding]::UTF8.GetBytes('{"items":[]}')
                break
            }
            "/mock/duplicate" {
                $response.StatusCode = 200
                $payload = '{"items":[{"url":"https://x.com/duplicate/status/1","content":"duplicate crypto item","author":"@mock","created_at":"2026-06-04T10:00:00Z"},{"url":"https://x.com/duplicate/status/1","content":"duplicate crypto item","author":"@mock","created_at":"2026-06-04T10:00:00Z"}]}'
                $buffer = [System.Text.Encoding]::UTF8.GetBytes($payload)
                break
            }
            default {
                $response.StatusCode = 404
                $buffer = [System.Text.Encoding]::UTF8.GetBytes('{"code":404,"message":"not found"}')
                break
            }
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
