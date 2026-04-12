[CmdletBinding()]
param(
    [string]$PythonExe = "python",
    [string]$SshHost = "tencent-opscaptain",
    [string]$DatasetRoot = "",
    [string]$OutputRoot = "",
    [string]$RemoteWorkspace = "/opt/opscaptain/baseline-workspace",
    [string]$Mode = "full",
    [int]$ProgressSeconds = 30,
    [int]$PollSeconds = 30,
    [string]$LogPath = "",
    [switch]$NoStartMilvus,
    [switch]$NoCleanLocalOutput,
    [switch]$KeepArchive,
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Log {
    param([string]$Message)
    $line = "[{0}] {1}" -f (Get-Date -Format "yyyy-MM-dd HH:mm:ss"), $Message
    Add-Content -Path $script:LogPath -Value $line -Encoding UTF8
    Write-Output $line
}

function Invoke-Checked {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [switch]$CaptureOutput
    )
    if ($DryRun) {
        Write-Log ("DRYRUN " + $FilePath + " " + ($Arguments -join " "))
        return ""
    }

    if ($CaptureOutput) {
        $output = & $FilePath @Arguments 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw ("command failed: " + $FilePath + " " + ($Arguments -join " ") + "`n" + ($output -join "`n"))
        }
        return ($output -join "`n")
    }

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw ("command failed: " + $FilePath + " " + ($Arguments -join " "))
    }
    return ""
}

function Stop-ExistingBuilder {
    $matches = Get-CimInstance Win32_Process |
        Where-Object {
            $_.Name -match '^python(?:\.exe)?$' -and
            $_.CommandLine -like '*build_telemetry_evidence.py*'
        }
    foreach ($item in $matches) {
        Write-Log ("stopping existing telemetry builder pid=" + $item.ProcessId)
        if (-not $DryRun) {
            Stop-Process -Id $item.ProcessId -Force
        }
    }
}

function Get-DocCount {
    param([string]$DirPath)
    if (-not (Test-Path $DirPath)) {
        return 0
    }
    return (Get-ChildItem -Path $DirPath -Filter *.md -File -ErrorAction SilentlyContinue | Measure-Object).Count
}

function Read-JsonFile {
    param([string]$Path)
    if (-not (Test-Path $Path)) {
        return $null
    }
    try {
        return (Get-Content -Path $Path -Raw -Encoding UTF8 | ConvertFrom-Json)
    } catch {
        return $null
    }
}

function Tail-File {
    param(
        [string]$Path,
        [int]$Lines = 20
    )
    if (-not (Test-Path $Path)) {
        return @()
    }
    return Get-Content -Path $Path -Tail $Lines -Encoding UTF8
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path

if (-not $DatasetRoot) {
    $DatasetRoot = Join-Path $repoRoot "aiopschallenge2025"
}
if (-not $OutputRoot) {
    $OutputRoot = Join-Path $DatasetRoot "baseline"
}

$logsDir = Join-Path $repoRoot "logs"
New-Item -ItemType Directory -Force -Path $logsDir | Out-Null

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
if (-not $LogPath) {
    $LogPath = Join-Path $logsDir ("telemetry-local-remote-{0}.log" -f $stamp)
}
$builderOutLog = Join-Path $logsDir ("telemetry-local-remote-{0}.builder.out.log" -f $stamp)
$builderErrLog = Join-Path $logsDir ("telemetry-local-remote-{0}.builder.err.log" -f $stamp)
$archivePath = Join-Path $logsDir ("telemetry-light-artifacts-{0}.tar.gz" -f $stamp)

$docsDir = Join-Path $OutputRoot "docs_evidence_telemetry"
$docsBuildDir = Join-Path $OutputRoot "docs_evidence_telemetry_build"
$telemetryDir = Join-Path $OutputRoot "telemetry"
$evalDir = Join-Path $OutputRoot "eval"
$reportPath = Join-Path $telemetryDir "telemetry_report.json"

$remoteDatasetRoot = "$RemoteWorkspace/aiopschallenge2025"
$remoteOutputRoot = "$remoteDatasetRoot/baseline"
$remoteArchivePath = "/tmp/telemetry-light-artifacts-$stamp.tar.gz"
$remoteEvalLog = "/opt/opscaptain/telemetry-light-baseline-$stamp.log"
$remoteEvalReport = "$remoteOutputRoot/eval/report_evidence_telemetry_build_related.json"

Write-Log "repo_root=$repoRoot"
Write-Log "dataset_root=$DatasetRoot"
Write-Log "output_root=$OutputRoot"
Write-Log "ssh_host=$SshHost"
Write-Log "remote_workspace=$RemoteWorkspace"
Write-Log "mode=$Mode"
Write-Log "progress_seconds=$ProgressSeconds"
Write-Log "poll_seconds=$PollSeconds"
Write-Log "log_path=$LogPath"

if (-not (Test-Path $DatasetRoot)) {
    throw "dataset root not found: $DatasetRoot"
}
if (-not (Test-Path (Join-Path $DatasetRoot "input.json"))) {
    throw "missing dataset file: $(Join-Path $DatasetRoot 'input.json')"
}
if (-not (Test-Path (Join-Path $DatasetRoot "groundtruth.jsonl"))) {
    throw "missing dataset file: $(Join-Path $DatasetRoot 'groundtruth.jsonl')"
}
if (-not (Test-Path (Join-Path $DatasetRoot "extracted"))) {
    throw "missing dataset directory: $(Join-Path $DatasetRoot 'extracted')"
}

Invoke-Checked -FilePath "ssh" -Arguments @($SshHost, "echo connected") | Out-Null

Stop-ExistingBuilder

if (-not $NoCleanLocalOutput) {
    Write-Log "cleaning previous telemetry output"
    foreach ($path in @($docsDir, $docsBuildDir, $telemetryDir)) {
        if ((Test-Path $path) -and (-not $DryRun)) {
            Remove-Item -LiteralPath $path -Recurse -Force
        }
    }
}

foreach ($path in @($docsDir, $docsBuildDir, $telemetryDir, $evalDir)) {
    if (-not (Test-Path $path)) {
        New-Item -ItemType Directory -Force -Path $path | Out-Null
    }
}

$builderArgs = @(
    (Join-Path $repoRoot "scripts\aiops\build_telemetry_evidence.py"),
    "--dataset-root", $DatasetRoot,
    "--output-root", $OutputRoot,
    "--progress-seconds", $ProgressSeconds
)

Write-Log "starting local telemetry preprocessing"
if ($DryRun) {
    Write-Log ("DRYRUN " + $PythonExe + " " + ($builderArgs -join " "))
} else {
    $builder = Start-Process `
        -FilePath $PythonExe `
        -ArgumentList $builderArgs `
        -WorkingDirectory $repoRoot `
        -RedirectStandardOutput $builderOutLog `
        -RedirectStandardError $builderErrLog `
        -PassThru `
        -WindowStyle Hidden

    Write-Log ("builder_pid=" + $builder.Id)
    while (-not $builder.HasExited) {
        Start-Sleep -Seconds $PollSeconds
        $docCount = Get-DocCount -DirPath $docsBuildDir
        $report = Read-JsonFile -Path $reportPath
        if ($null -ne $report) {
            Write-Log (
                "local_progress cases={0}/{1} build_cases={2} docs_build={3} empty={4} current={5} elapsed={6}s" -f
                $report.cases, $report.total_cases, $report.build_cases, $docCount, $report.empty_cases, $report.current_case_id, $report.elapsed_seconds
            )
        } else {
            Write-Log ("local_progress docs_build=" + $docCount)
        }
    }

    $builder.WaitForExit()
    if ($builder.ExitCode -ne 0) {
        $stderrTail = (Tail-File -Path $builderErrLog -Lines 40) -join "`n"
        throw ("telemetry builder failed, exit_code={0}`n{1}" -f $builder.ExitCode, $stderrTail)
    }
}

$finalReport = Read-JsonFile -Path $reportPath
if ($null -eq $finalReport -and -not $DryRun) {
    throw "telemetry_report.json not found after preprocessing"
}
if (-not $DryRun) {
    Write-Log (
        "local_completed cases={0}/{1} build_cases={2} docs_build={3} metric={4} log={5} trace={6}" -f
        $finalReport.cases,
        $finalReport.total_cases,
        $finalReport.build_cases,
        (Get-DocCount -DirPath $docsBuildDir),
        $finalReport.metric_signals,
        $finalReport.log_signals,
        $finalReport.trace_signals
    )
}

Write-Log "packing lightweight telemetry artifacts"
Invoke-Checked -FilePath "tar" -Arguments @(
    "-czf", $archivePath,
    "-C", $repoRoot,
    "aiopschallenge2025/baseline/docs_evidence_telemetry_build",
    "aiopschallenge2025/baseline/telemetry",
    "aiopschallenge2025/baseline/eval"
)

$archiveSize = if (Test-Path $archivePath) { (Get-Item $archivePath).Length } else { 0 }
Write-Log ("archive_path=" + $archivePath + " bytes=" + $archiveSize)

Write-Log "preparing remote workspace for lightweight artifact upload"
Invoke-Checked -FilePath "ssh" -Arguments @(
    $SshHost,
    "mkdir -p '$remoteOutputRoot' && rm -rf '$remoteOutputRoot/docs_evidence_telemetry_build' '$remoteOutputRoot/telemetry' '$remoteOutputRoot/eval'"
)

Write-Log "uploading lightweight archive to remote"
Invoke-Checked -FilePath "scp" -Arguments @($archivePath, "${SshHost}:$remoteArchivePath")

Write-Log "extracting lightweight archive on remote"
Invoke-Checked -FilePath "ssh" -Arguments @(
    $SshHost,
    "cd '$RemoteWorkspace' && tar -xzf '$remoteArchivePath' && rm -f '$remoteArchivePath'"
)

$remoteRunnerArgs = @()
if (-not $NoStartMilvus) {
    $remoteRunnerArgs += "--start-milvus"
}
$remoteRunnerArgs += @("--skip-prep", "--skip-telemetry", "--mode", $Mode)
$remoteRunnerArgString = [string]::Join(" ", $remoteRunnerArgs)
$startRemoteCmd = "cd '$RemoteWorkspace' && nohup bash scripts/aiops/run_telemetry_baseline_remote.sh $remoteRunnerArgString > '$remoteEvalLog' 2>&1 & echo `$!"

Write-Log "starting remote indexing and evaluation"
$remotePid = (Invoke-Checked -FilePath "ssh" -Arguments @($SshHost, $startRemoteCmd) -CaptureOutput).Trim()
if (-not $remotePid) {
    throw "failed to capture remote evaluation pid"
}
Write-Log ("remote_eval_pid=" + $remotePid + " remote_log=" + $remoteEvalLog)

if (-not $DryRun) {
    while ($true) {
        Start-Sleep -Seconds $PollSeconds
        $remoteProbe = @"
status="unknown"
if kill -0 $remotePid 2>/dev/null; then
  status="running"
else
  status="exited"
fi
printf '__STATUS__=%s\n' "$status"
if [ -f "$remoteEvalReport" ]; then
  printf '__REPORT__=present\n'
else
  printf '__REPORT__=missing\n'
fi
tail -n 12 "$remoteEvalLog" 2>/dev/null || true
"@
        $probeOutput = Invoke-Checked -FilePath "ssh" -Arguments @($SshHost, $remoteProbe) -CaptureOutput
        $lines = @()
        if ($probeOutput) {
            $lines = $probeOutput -split "`r?`n"
        }

        $statusLine = ($lines | Where-Object { $_ -like "__STATUS__=*" } | Select-Object -First 1)
        $reportLine = ($lines | Where-Object { $_ -like "__REPORT__=*" } | Select-Object -First 1)
        $status = if ($statusLine) { $statusLine.Split("=", 2)[1] } else { "unknown" }
        $reportState = if ($reportLine) { $reportLine.Split("=", 2)[1] } else { "missing" }
        $tailLines = $lines | Where-Object { $_ -notlike "__STATUS__=*" -and $_ -notlike "__REPORT__=*" }

        Write-Log ("remote_progress status=" + $status + " report=" + $reportState)
        if ($tailLines.Count -gt 0) {
            $startIndex = [Math]::Max(0, $tailLines.Count - 6)
            for ($i = $startIndex; $i -lt $tailLines.Count; $i++) {
                $line = $tailLines[$i]
                if ($line) {
                    Write-Log ("remote_log " + $line)
                }
            }
        }

        if ($status -eq "exited") {
            break
        }
    }
}

Write-Log "fetching remote evaluation summary"
$summaryCmd = @"
python3 - <<'PY'
import json
from pathlib import Path
path = Path("$remoteEvalReport")
if not path.exists():
    raise SystemExit("missing report: " + str(path))
data = json.loads(path.read_text(encoding="utf-8"))
summary = data.get("summary", {})
print(json.dumps({
    "cases": summary.get("cases"),
    "avg_total_latency_ms": summary.get("avg_total_latency_ms"),
    "empty_rate": summary.get("empty_rate"),
    "hit_rate_at_k": summary.get("hit_rate_at_k"),
    "avg_recall_at_k": summary.get("avg_recall_at_k"),
}, ensure_ascii=False))
PY
"@
$summaryOutput = Invoke-Checked -FilePath "ssh" -Arguments @($SshHost, $summaryCmd) -CaptureOutput
Write-Log ("remote_summary " + $summaryOutput.Trim())

if (-not $KeepArchive -and -not $DryRun -and (Test-Path $archivePath)) {
    Remove-Item -LiteralPath $archivePath -Force
    Write-Log "removed local archive"
}

Write-Log "pipeline completed"
