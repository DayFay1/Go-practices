param(
	[string]$BaseUrl = "http://localhost:8080",
	[string]$AdminEmail = "admin@example.com",
	[string]$AdminPassword = "admin123",
	[switch]$BuildApi,
	[switch]$SkipDocker
)

$ErrorActionPreference = "Stop"

function Assert-Equal {
	param(
		[Parameter(Mandatory)]$Actual,
		[Parameter(Mandatory)]$Expected,
		[Parameter(Mandatory)][string]$Message
	)
	if ($Actual -ne $Expected) {
		throw "$Message (expected $Expected, got $Actual)"
	}
}

Add-Type -AssemblyName System.Net.Http

function Get-HttpMethod {
	param([Parameter(Mandatory)][string]$Method)
	switch ($Method.ToUpperInvariant()) {
		"GET" { return [System.Net.Http.HttpMethod]::Get }
		"POST" { return [System.Net.Http.HttpMethod]::Post }
		"PUT" { return [System.Net.Http.HttpMethod]::Put }
		"DELETE" { return [System.Net.Http.HttpMethod]::Delete }
		"PATCH" { return [System.Net.Http.HttpMethod]::new("PATCH") }
		default { throw "Unsupported method: $Method" }
	}
}

$client = [System.Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(30)

function Invoke-Http {
	param(
		[Parameter(Mandatory)][string]$Method,
		[Parameter(Mandatory)][string]$Url,
		[hashtable]$Headers = @{},
		$Body = $null
	)

	$req = [System.Net.Http.HttpRequestMessage]::new((Get-HttpMethod $Method), $Url)
	foreach ($k in $Headers.Keys) {
		[void]$req.Headers.TryAddWithoutValidation($k, [string]$Headers[$k])
	}

	if ($null -ne $Body) {
		$json = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 10 }
		$req.Content = [System.Net.Http.StringContent]::new($json, [System.Text.Encoding]::UTF8, "application/json")
	}

	$resp = $client.SendAsync($req).GetAwaiter().GetResult()
	$content = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()

	return [pscustomobject]@{
		StatusCode = [int]$resp.StatusCode
		Content    = $content
	}
}

function Wait-Ready {
	param([int]$TimeoutSeconds = 120)

	$deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
	while ([DateTime]::UtcNow -lt $deadline) {
		$resp = Invoke-Http -Method "GET" -Url "$BaseUrl/ready"
		if ($resp.StatusCode -eq 200) {
			return
		}
		Start-Sleep -Seconds 1
	}
	throw "API not ready at $BaseUrl within $TimeoutSeconds seconds"
}

function Invoke-DockerCompose {
	param([Parameter(Mandatory)][string[]]$Args)
	& docker compose @Args | Out-Host
	if ($LASTEXITCODE -ne 0) {
		throw "docker compose $($Args -join ' ') failed with exit code $LASTEXITCODE"
	}
}

try {
	if (-not $SkipDocker) {
		Invoke-DockerCompose -Args @("up", "-d", "db")
		Invoke-DockerCompose -Args @("run", "--rm", "migrate", "up")
		if ($BuildApi) {
			Invoke-DockerCompose -Args @("up", "-d", "--build", "api")
		} else {
			Invoke-DockerCompose -Args @("up", "-d", "api")
		}
	}

	Write-Host "Waiting for readiness..."
	Wait-Ready

	Write-Host "GET /health"
	$health = Invoke-Http -Method "GET" -Url "$BaseUrl/health"
	Assert-Equal $health.StatusCode 200 "health status"
	$healthJson = $health.Content | ConvertFrom-Json
	Assert-Equal $healthJson.status "ok" "health body"

	Write-Host "GET /metrics"
	$metrics = Invoke-Http -Method "GET" -Url "$BaseUrl/metrics"
	Assert-Equal $metrics.StatusCode 200 "metrics status"
	if ($metrics.Content -notmatch "polling_http_requests_total") {
		throw "metrics endpoint doesn't expose polling_http_requests_total"
	}

	Write-Host "POST /auth/login (admin)"
	$adminLogin = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/auth/login" -Body @{
		email    = $AdminEmail
		password = $AdminPassword
	}
	Assert-Equal $adminLogin.StatusCode 200 "admin login status"
	$adminLoginJson = $adminLogin.Content | ConvertFrom-Json
	$adminToken = $adminLoginJson.token
	if ([string]::IsNullOrWhiteSpace($adminToken)) {
		throw "admin token is empty"
	}

	$ts = Get-Date -Format "yyyyMMddHHmmss"
	$userEmail = "smoke-$ts@example.com"
	$userPassword = "smoke-pass-$ts"

	Write-Host "POST /auth/register (user: $userEmail)"
	$register = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/auth/register" -Body @{
		email    = $userEmail
		password = $userPassword
	}
	Assert-Equal $register.StatusCode 201 "register status"
	$registerJson = $register.Content | ConvertFrom-Json
	$userToken = $registerJson.token
	$userID = [int64]$registerJson.user.id
	if ([string]::IsNullOrWhiteSpace($userToken)) {
		throw "user token is empty"
	}
	if ($userID -le 0) {
		throw "invalid user id: $userID"
	}

	Write-Host "POST /polls (should be forbidden for user)"
	$userCreatePoll = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/polls" -Headers @{
		Authorization = "Bearer $userToken"
	} -Body @{
		title   = "should not work"
		options = @("A", "B")
	}
	Assert-Equal $userCreatePoll.StatusCode 403 "user create poll status"

	Write-Host "POST /polls (admin)"
	$createPoll = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/polls" -Headers @{
		Authorization = "Bearer $adminToken"
	} -Body @{
		title   = "Smoke poll $ts"
		options = @("Option A", "Option B")
	}
	Assert-Equal $createPoll.StatusCode 201 "create poll status"
	$pollID = [int64]((($createPoll.Content | ConvertFrom-Json).id))
	if ($pollID -le 0) {
		throw "invalid poll id: $pollID"
	}

	Write-Host "PATCH /polls/$pollID (admin)"
	$updatePoll = Invoke-Http -Method "PATCH" -Url "$BaseUrl/api/v1/polls/$pollID" -Headers @{
		Authorization = "Bearer $adminToken"
	} -Body @{
		title = "Smoke poll updated $ts"
	}
	Assert-Equal $updatePoll.StatusCode 204 "update poll status"

	Write-Host "PATCH /polls/$pollID/status => active (admin)"
	$activate = Invoke-Http -Method "PATCH" -Url "$BaseUrl/api/v1/polls/$pollID/status" -Headers @{
		Authorization = "Bearer $adminToken"
	} -Body @{
		status = "active"
	}
	Assert-Equal $activate.StatusCode 204 "activate poll status"

	Write-Host "GET /polls/$pollID (user)"
	$getPoll = Invoke-Http -Method "GET" -Url "$BaseUrl/api/v1/polls/$pollID" -Headers @{
		Authorization = "Bearer $userToken"
	}
	Assert-Equal $getPoll.StatusCode 200 "get poll status"
	$getPollJson = $getPoll.Content | ConvertFrom-Json
	$optionID = [int64]$getPollJson.options[0].id
	if ($optionID -le 0) {
		throw "invalid option id: $optionID"
	}

	Write-Host "POST /polls/$pollID/vote (user)"
	$vote = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/polls/$pollID/vote" -Headers @{
		Authorization = "Bearer $userToken"
	} -Body @{
		option_id = $optionID
	}
	Assert-Equal $vote.StatusCode 204 "vote status"

	Write-Host "POST /polls/$pollID/vote again (should be 409)"
	$voteAgain = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/polls/$pollID/vote" -Headers @{
		Authorization = "Bearer $userToken"
	} -Body @{
		option_id = $optionID
	}
	Assert-Equal $voteAgain.StatusCode 409 "duplicate vote status"

	Write-Host "GET /polls/$pollID/results (user)"
	$results = Invoke-Http -Method "GET" -Url "$BaseUrl/api/v1/polls/$pollID/results" -Headers @{
		Authorization = "Bearer $userToken"
	}
	Assert-Equal $results.StatusCode 200 "results status"
	$resultsJson = $results.Content | ConvertFrom-Json
	Assert-Equal ([int64]$resultsJson.total_votes) 1 "results total_votes"

	Write-Host "PATCH /users/$userID/deactivate (admin)"
	$deactivate = Invoke-Http -Method "PATCH" -Url "$BaseUrl/api/v1/users/$userID/deactivate" -Headers @{
		Authorization = "Bearer $adminToken"
	} -Body @{}
	Assert-Equal $deactivate.StatusCode 204 "deactivate status"

	Write-Host "POST /auth/login (deactivated user should be 401)"
	$userLoginAfterDeactivate = Invoke-Http -Method "POST" -Url "$BaseUrl/api/v1/auth/login" -Body @{
		email    = $userEmail
		password = $userPassword
	}
	Assert-Equal $userLoginAfterDeactivate.StatusCode 401 "deactivated user login status"

	Write-Host "DELETE /polls/$pollID (admin)"
	$deletePoll = Invoke-Http -Method "DELETE" -Url "$BaseUrl/api/v1/polls/$pollID" -Headers @{
		Authorization = "Bearer $adminToken"
	}
	Assert-Equal $deletePoll.StatusCode 204 "delete poll status"

	Write-Host "SMOKE TEST OK"
	exit 0
} catch {
	Write-Error $_
	exit 1
}
