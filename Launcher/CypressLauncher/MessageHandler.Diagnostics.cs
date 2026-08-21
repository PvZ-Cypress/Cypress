#nullable enable
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Net.Http;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;

namespace CypressLauncher;

public partial class MessageHandler
{
	private readonly List<Process> m_diagnosticProcesses = new();
	private readonly Queue<string> m_cfb27RecentEvents = new();
	private readonly object m_cfb27RecentEventsLock = new();
	private CFB27DiscoveryCapture? m_cfb27Capture;
	private CFB27EAmitmImport? m_cfb27EAmitmImport;
	private CFB27EndpointExperiment? m_cfb27EndpointExperiment;
	private string m_cfb27PrivateRunDirectory = "";
	private string m_cfb27PrivateProfile = "LocalPlayer";
	private string m_cfb27PrivateStartLog = "";
	private string m_cfb27BridgeConfigPath = "";

	private async Task<string> EnsureCFB27PrivateStackAsync(string profile)
	{
		string root = FindCypressRoot();
		string servicesDir = Path.Combine(root, "tools", "cypress-servers");
		string buildDir = Path.Combine(servicesDir, "build");
		string dynastyExe = FindToolExe(servicesDir, buildDir, "dynasty.exe");
		string blazeExe = FindToolExe(servicesDir, buildDir, "cfb27blaze.exe");
		string privateRoot = Path.Combine(GetAppdataDir(), "CFB27", "Private");
		string dataDir = Path.Combine(privateRoot, "data");
		string schemaRoot = FindCFB27SchemaRoot(servicesDir, dataDir);
		string dynastySeed = FindCFB27DynastySeed();
		string runDir = Path.Combine(privateRoot, "runs", DateTime.Now.ToString("yyyyMMdd_HHmmss"));
		Directory.CreateDirectory(dataDir);
		Directory.CreateDirectory(runDir);

		m_cfb27PrivateProfile = string.IsNullOrWhiteSpace(profile) ? "LocalPlayer" : profile.Trim();
		m_cfb27PrivateRunDirectory = runDir;
		m_cfb27PrivateStartLog = Path.Combine(runDir, "private-start.log");
		m_cfb27BridgeConfigPath = Path.Combine(privateRoot, "cfb27-bridge.ini");
		WriteCFB27BridgeConfig(m_cfb27BridgeConfigPath, runDir, m_cfb27PrivateProfile);
		LogCFB27PrivateStart("private stack start requested");
		LogCFB27PrivateStart("root=" + root);
		LogCFB27PrivateStart("servicesDir=" + servicesDir);
		LogCFB27PrivateStart("dynastyExe=" + dynastyExe);
		LogCFB27PrivateStart("blazeExe=" + blazeExe);
		LogCFB27PrivateStart("schemaRoot=" + schemaRoot);
		LogCFB27PrivateStart("dynastySeed=" + dynastySeed);
		LogCFB27PrivateStart("dataDir=" + dataDir);
		LogCFB27PrivateStart("profile=" + m_cfb27PrivateProfile);
		string assetCatalog = await ExportCFB27AssetCatalogAsync(servicesDir, dataDir, schemaRoot);
		LogCFB27PrivateStart("assetCatalog=" + assetCatalog);
		SendStatus("Starting CFB27 private Dynasty service...", "info");

		await EnsureDiagnosticProcessAsync(
			"dynasty",
			dynastyExe,
			BuildCFB27DynastyArguments(schemaRoot, dataDir, servicesDir, dynastySeed, "node"),
			runDir,
			"http://127.0.0.1:27910/health",
			startupTimeoutSeconds: 300,
			restartIfHealthy: true);
		await RequireCFB27DynastyPersistenceAsync("http://127.0.0.1:27910/health");
		LogCFB27PrivateStart("dynasty service healthy");
		SendStatus("Starting CFB27 local Blaze bridge...", "info");

		await EnsureDiagnosticProcessAsync(
			"cfb27blaze",
			blazeExe,
			BuildCFB27BlazeArguments(m_cfb27PrivateProfile, runDir, assetCatalog),
			runDir,
			"http://127.0.0.1:27921/health",
			startupTimeoutSeconds: 30,
			restartIfHealthy: true);
		LogCFB27PrivateStart("cfb27blaze service healthy");

		RecordCFB27Event($"private stack ready runDir={runDir} profile={m_cfb27PrivateProfile}");
		LogCFB27PrivateStart("private stack ready");
		return runDir;
	}

	private void LogCFB27PrivateStart(string line)
	{
		if (string.IsNullOrWhiteSpace(m_cfb27PrivateStartLog))
			return;
		try
		{
			File.AppendAllText(
				m_cfb27PrivateStartLog,
				$"{DateTime.Now:O} {line}{Environment.NewLine}");
		}
		catch { }
	}

	private static string FindCFB27SchemaRoot(string servicesDir, string dataDir)
	{
		return SelectCFB27SchemaRoot(
			servicesDir,
			dataDir,
			Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory));
	}

	private static string SelectCFB27SchemaRoot(string servicesDir, string dataDir, string desktopDirectory)
	{
		string releaseAssets0 = Path.Combine(
			desktopDirectory,
			"CFB27",
			"Release",
			"Dynasty_Assets",
			"0");
		if (Directory.Exists(releaseAssets0))
			return releaseAssets0;

		string packaged = Path.Combine(servicesDir, "deploy", "Dynasty_Files");
		if (Directory.Exists(packaged))
			return packaged;

		string desktop = Path.Combine(
			desktopDirectory,
			"CFB27",
			"Dynasty_Files");
		if (Directory.Exists(desktop))
			return desktop;

		string releaseAssets = Path.Combine(
			desktopDirectory,
			"CFB27",
			"Release",
			"Dynasty_Assets",
			"2");
		if (Directory.Exists(releaseAssets))
			return releaseAssets;

		string emptySchemaRoot = Path.Combine(dataDir, "Dynasty_Files");
		Directory.CreateDirectory(emptySchemaRoot);
		return emptySchemaRoot;
	}

	private static string FindCFB27DynastySeed()
	{
		return SelectCFB27DynastySeed(
			Environment.GetFolderPath(Environment.SpecialFolder.MyDocuments));
	}

	private static string SelectCFB27DynastySeed(string documentsDirectory)
	{
		string savesDirectory = Path.Combine(
			documentsDirectory,
			"EA SPORTS College Football 27",
			"Saves");
		if (!Directory.Exists(savesDirectory))
			throw new DirectoryNotFoundException(
				$"CFB27 private Dynasty needs one full offline Dynasty save. The saves directory was not found: '{savesDirectory}'.");

		string? selected = Directory
			.EnumerateFiles(savesDirectory, "*", SearchOption.TopDirectoryOnly)
			.Where(IsCFB27FullDynastySave)
			.Where(file => Path.GetFileName(file).StartsWith("DYNASTY", StringComparison.OrdinalIgnoreCase))
			.OrderByDescending(File.GetLastWriteTimeUtc)
			.FirstOrDefault();
		if (string.IsNullOrWhiteSpace(selected))
			throw new FileNotFoundException(
				$"CFB27 private Dynasty needs a DYNASTY* FBCHUNKS offline save in '{savesDirectory}'.");
		return Path.GetFullPath(selected);
	}

	private static bool IsCFB27FullDynastySave(string file)
	{
		try
		{
			using var stream = File.OpenRead(file);
			Span<byte> header = stackalloc byte[8];
			return stream.Read(header) == header.Length &&
				Encoding.ASCII.GetString(header) == "FBCHUNKS";
		}
		catch
		{
			return false;
		}
	}

	private static string[] BuildCFB27DynastyArguments(
		string schemaRoot,
		string dataDir,
		string servicesDir,
		string dynastySeed,
		string nodePath)
	{
		return new[]
		{
			"-bind", "127.0.0.1",
			"-port", "27910",
			"-schema-root", schemaRoot,
			"-db", Path.Combine(dataDir, "cfb27_dynasty.db"),
			"-seed", dynastySeed,
			"-data-dir", Path.Combine(dataDir, "dynasties"),
			"-node", nodePath,
			"-franchise-tool", Path.Combine(servicesDir, "cmd", "cfb27franchise", "main.mjs")
		};
	}

	private static string[] BuildCFB27BlazeArguments(string profile, string runDir, string assetCatalog)
	{
		return new[]
		{
			"-bind", "127.0.0.1",
			"-port", "27920",
			"-diagnostics-bind", "127.0.0.1",
			"-diagnostics-port", "27921",
			"-dynasty-url", "http://127.0.0.1:27910",
			"-profile", profile,
			"-run-id", Path.GetFileName(runDir),
			"-log-file", Path.Combine(runDir, "cfb27-blaze.jsonl"),
			"-coach-catalog", assetCatalog
		};
	}

	private static void ValidateCFB27DynastyHealth(string json)
	{
		JObject health;
		try
		{
			health = JObject.Parse(json);
		}
		catch (JsonException ex)
		{
			throw new InvalidOperationException("CFB27 Dynasty service returned invalid health data.", ex);
		}
		if (health.Value<string>("status") != "ok" || health.Value<bool?>("artifactConfigured") != true)
			throw new InvalidOperationException(
				"CFB27 Dynasty service is running without FBCHUNKS franchise persistence.");
	}

	private static async Task RequireCFB27DynastyPersistenceAsync(string healthUrl)
	{
		using var response = await s_httpClient.GetAsync(healthUrl);
		response.EnsureSuccessStatusCode();
		ValidateCFB27DynastyHealth(await response.Content.ReadAsStringAsync());
	}

	private async Task<string> ExportCFB27AssetCatalogAsync(string servicesDir, string dataDir, string schemaRoot)
	{
		string normalizedSchemaRoot = Path.TrimEndingDirectorySeparator(Path.GetFullPath(schemaRoot));
		string slotName = Path.GetFileName(normalizedSchemaRoot);
		if (!int.TryParse(slotName, out int slot) || slot < 0)
			throw new InvalidOperationException(
				$"CFB27 private Dynasty requires a numbered Dynasty_Assets slot; selected schema root was '{schemaRoot}'.");
		string? assetRoot = Directory.GetParent(normalizedSchemaRoot)?.FullName;
		if (string.IsNullOrWhiteSpace(assetRoot))
			throw new InvalidOperationException($"Could not resolve the Dynasty_Assets root from '{schemaRoot}'.");

		string dynastyAsset = Path.Combine(normalizedSchemaRoot, "dynasty-dynasty-binary.FTC");
		string mainSchema = Path.Combine(normalizedSchemaRoot, "franchise-schemas.FTX");
		if (!File.Exists(dynastyAsset) || !File.Exists(mainSchema))
			throw new FileNotFoundException(
				$"CFB27 Dynasty asset slot {slot} is incomplete. Expected '{dynastyAsset}' and '{mainSchema}'.");

		string exporter = Path.Combine(servicesDir, "cmd", "cfb27assetexport", "main.mjs");
		if (!File.Exists(exporter))
			throw new FileNotFoundException("CFB27 asset exporter is missing.", exporter);
		string dependency = Path.Combine(servicesDir, "node_modules", "madden-franchise", "package.json");
		if (!File.Exists(dependency))
		{
			LogCFB27PrivateStart("asset exporter dependencies missing; running npm install");
			await RunCFB27AssetToolAsync(
				"npm.cmd",
				new[] { "install", "--omit=dev", "--ignore-scripts", "--no-audit", "--no-fund" },
				servicesDir,
				"install CFB27 asset exporter dependencies");
		}

		string output = Path.Combine(dataDir, $"cfb27-assets-slot{slot}.json");
		await RunCFB27AssetToolAsync(
			"node",
			new[]
			{
				exporter,
				"--asset-root", assetRoot,
				"--slot", slot.ToString(),
				"--output", output
			},
			servicesDir,
			"export CFB27 coach, team, and player assets");
		if (!File.Exists(output))
			throw new FileNotFoundException("CFB27 asset exporter completed without writing its catalog.", output);
		return output;
	}

	private async Task RunCFB27AssetToolAsync(
		string executable,
		IEnumerable<string> arguments,
		string workingDirectory,
		string description)
	{
		var startInfo = new ProcessStartInfo
		{
			FileName = executable,
			WorkingDirectory = workingDirectory,
			UseShellExecute = false,
			CreateNoWindow = true,
			RedirectStandardOutput = true,
			RedirectStandardError = true
		};
		foreach (string argument in arguments)
			startInfo.ArgumentList.Add(argument);
		using Process process = Process.Start(startInfo)
			?? throw new InvalidOperationException($"Failed to {description}: could not start {executable}.");
		Task<string> stdoutTask = process.StandardOutput.ReadToEndAsync();
		Task<string> stderrTask = process.StandardError.ReadToEndAsync();
		await process.WaitForExitAsync();
		string stdout = await stdoutTask;
		string stderr = await stderrTask;
		if (!string.IsNullOrWhiteSpace(stdout))
			LogCFB27PrivateStart($"{description}: {stdout.Trim()}");
		if (process.ExitCode != 0)
			throw new InvalidOperationException(
				$"Failed to {description}; {executable} exited with code {process.ExitCode}. {stderr}".Trim());
	}

	private static void WriteCFB27BridgeConfig(string path, string runDir, string profile)
	{
		File.WriteAllLines(path, new[]
		{
			"# Written by CypressLauncher",
			"privateOnlineDynasty=true",
			$"privateLaunchExpiresUtc={DateTimeOffset.UtcNow.AddMinutes(10).ToUnixTimeSeconds()}",
			"externalPassThrough=false",
			"blazeHost=127.0.0.1",
			"blazePort=27920",
			$"profile={profile}",
			$"runDirectory={runDir}",
			"enableBearSslBypass=true",
			"dumpRuntimeCodeBytes=false",
			"enableCandidateEndpointRedirects=false",
			"enableProtoSslVerifyProbe=false",
			"enableCertVerifyHook=true",
			"certVerifyForce=true",
			"enableFailStateWatch=true"
		});
	}

	private void OnCFB27Diagnostics()
	{
		_ = Task.Run(async () =>
		{
			var result = new JObject
			{
				["type"] = "cfb27DiagnosticsResult",
				["ok"] = false,
				["masterUrl"] = "http://127.0.0.1:27900",
				["relayAddress"] = "127.0.0.1:25201",
				["dynastyUrl"] = "http://127.0.0.1:27910",
				["gatewayUrl"] = "http://127.0.0.1:27920"
			};

			try
			{
				if (!IsCFB27(m_selectedGame))
				{
					result["error"] = "Select CFB27 before running diagnostics.";
					Send(result);
					return;
				}

				string root = FindCypressRoot();
				string servicesDir = Path.Combine(root, "tools", "cypress-servers");
				string buildDir = Path.Combine(servicesDir, "build");
				string masterExe = FindToolExe(servicesDir, buildDir, "master.exe");
				string relayExe = FindToolExe(servicesDir, buildDir, "relay.exe");
				string dynastyExe = FindToolExe(servicesDir, buildDir, "dynasty.exe");
				string gatewayExe = FindToolExe(servicesDir, buildDir, "cfb27gateway.exe");
				string dataDir = Path.Combine(GetAppdataDir(), "Diagnostics", "CFB27");
				Directory.CreateDirectory(dataDir);
				string packagedCandidates = Path.Combine(servicesDir, "deploy", "cfb27-endpoints.example.json");
				string runtimeCandidates = Path.Combine(dataDir, "cfb27-endpoints.json");
				if (File.Exists(packagedCandidates))
					File.Copy(packagedCandidates, runtimeCandidates, overwrite: true);

				await EnsureDiagnosticProcessAsync("master", masterExe, new[]
				{
					"-bind", "127.0.0.1",
					"-port", "27900",
					"-db", Path.Combine(dataDir, "cypress_master.db"),
					"-secret-file", Path.Combine(dataDir, "moderator_secret.txt")
				}, dataDir, "http://127.0.0.1:27900/health");

				await EnsureDiagnosticProcessAsync("relay", relayExe, new[]
				{
					"-bind", "127.0.0.1",
					"-port", "25201",
					"-api-bind", "127.0.0.1",
					"-api-port", "8080",
					"-relay-host", "127.0.0.1",
					"-lease-file", Path.Combine(dataDir, "relay_leases.json"),
					"-log-file", Path.Combine(dataDir, "relay.log"),
					"-master-url", "http://127.0.0.1:27900",
					"-no-dashboard"
				}, dataDir, "http://127.0.0.1:8080/api/relays");

				await EnsureDiagnosticProcessAsync("dynasty", dynastyExe, new[]
				{
					"-bind", "127.0.0.1",
					"-port", "27910",
					"-schema-root", Path.Combine(servicesDir, "deploy", "Dynasty_Files"),
					"-db", Path.Combine(dataDir, "cfb27_dynasty.db")
				}, dataDir, "http://127.0.0.1:27910/health");

				await EnsureDiagnosticProcessAsync("cfb27gateway", gatewayExe, new[]
				{
					"-bind", "127.0.0.1",
					"-port", "27920",
					"-log-file", Path.Combine(dataDir, "cfb27_gateway.log"),
					"-candidates-file", Path.Combine(dataDir, "cfb27-endpoints.json")
				}, dataDir, "http://127.0.0.1:27920/health");

				await Task.Delay(1200);
				var heartbeat = new JObject
				{
					["game"] = "CFB27",
					["address"] = "127.0.0.1",
					["port"] = 25201,
					["players"] = 0,
					["maxPlayers"] = 32,
					["motd"] = "CFB27 Diagnostics",
					["level"] = "CFB27_Dynasty",
					["mode"] = "OnlineDynasty",
					["dynastyMode"] = "Online Dynasty",
					["leagueName"] = "Diagnostics League",
					["currentStage"] = "diagnostics",
					["teamCount"] = 0,
					["rosterModded"] = false,
					["relayAddress"] = "127.0.0.1:25201"
				};
				using var body = new StringContent(heartbeat.ToString(Formatting.None), Encoding.UTF8, "application/json");
				using var resp = await s_httpClient.PostAsync("http://127.0.0.1:27900/heartbeat", body);
				string text = await resp.Content.ReadAsStringAsync();
				result["heartbeatStatus"] = (int)resp.StatusCode;
				result["heartbeatResponse"] = text;
				result["ok"] = resp.IsSuccessStatusCode;
				if (!resp.IsSuccessStatusCode)
					result["error"] = "Local master rejected the diagnostic heartbeat.";
				result["services"] = await GetCFB27Capture().CaptureAsync("diagnostics service probe", m_gameDirectory, GetCFB27CaptureInstances(), GetCFB27RecentEvents())
					.ContinueWith(t => new JObject { ["evidencePath"] = t.Result.RunDirectory, ["evidenceOk"] = t.Result.Ok });
			}
			catch (Exception ex)
			{
				result["error"] = ex.Message;
			}
			Send(result);
		});
	}

	private void OnCFB27CaptureSnapshot(JObject msg)
	{
		_ = Task.Run(async () =>
		{
			string scenario = ((string?)msg["scenario"]) ?? "manual snapshot";
			var result = await GetCFB27Capture().CaptureAsync(
				scenario,
				m_gameDirectory,
				GetCFB27CaptureInstances(),
				GetCFB27RecentEvents());
			Send(new JObject
			{
				["type"] = "cfb27CaptureResult",
				["ok"] = result.Ok,
				["runId"] = result.RunId,
				["path"] = result.RunDirectory,
				["error"] = result.Error ?? ""
			});
		});
	}

	private void OnCFB27TraceEndpoints(JObject msg)
	{
		_ = Task.Run(async () =>
		{
			int seconds = (int?)msg["seconds"] ?? 30;
			var result = await GetCFB27Capture().TraceLiveEndpointsAsync(seconds);
			Send(new JObject
			{
				["type"] = "cfb27TraceResult",
				["ok"] = result.Ok,
				["runId"] = result.RunId,
				["path"] = result.RunDirectory,
				["eventCount"] = result.EventCount,
				["error"] = result.Error ?? ""
			});
		});
	}

	private void OnCFB27NetworkTrace(JObject msg)
	{
		_ = Task.Run(async () =>
		{
			int seconds = (int?)msg["seconds"] ?? 30;
			var result = await GetCFB27EndpointExperiment().CaptureWindowsTraceAsync(seconds);
			Send(new JObject
			{
				["type"] = "cfb27ExperimentResult",
				["action"] = "networkTrace",
				["ok"] = result.Ok,
				["runId"] = result.RunId,
				["path"] = result.Path,
				["message"] = result.Message,
				["error"] = result.Error ?? ""
			});
		});
	}

	private void OnCFB27BlockCandidates()
	{
		_ = Task.Run(async () =>
		{
			var result = await GetCFB27EndpointExperiment().BlockCandidatesAsync();
			Send(new JObject
			{
				["type"] = "cfb27ExperimentResult",
				["action"] = "blockCandidates",
				["ok"] = result.Ok,
				["runId"] = result.RunId,
				["path"] = result.Path,
				["message"] = result.Message,
				["error"] = result.Error ?? ""
			});
		});
	}

	private void OnCFB27UnblockCandidates()
	{
		_ = Task.Run(async () =>
		{
			var result = await GetCFB27EndpointExperiment().UnblockCandidatesAsync();
			Send(new JObject
			{
				["type"] = "cfb27ExperimentResult",
				["action"] = "unblockCandidates",
				["ok"] = result.Ok,
				["runId"] = result.RunId,
				["path"] = result.Path,
				["message"] = result.Message,
				["error"] = result.Error ?? ""
			});
		});
	}

	private void OnCFB27OpenEvidenceFolder()
	{
		try
		{
			string path = GetCFB27Capture().EvidenceRoot;
			Directory.CreateDirectory(path);
			Process.Start(new ProcessStartInfo { FileName = path, UseShellExecute = true });
		}
		catch (Exception ex)
		{
			SendStatus("Failed to open evidence folder: " + ex.Message, "error");
		}
	}

	private void OnCFB27ImportEAmitm()
	{
		_ = Task.Run(async () =>
		{
			try
			{
				var result = await GetCFB27EAmitmImport().ImportLatestAsync(FindCypressRoot());
				Send(new JObject
				{
					["type"] = "cfb27EAmitmImportResult",
					["ok"] = result.Ok,
					["source"] = result.Source ?? "",
					["path"] = result.Report ?? GetCFB27EAmitmImport().EvidenceRoot,
					["bytes"] = result.Bytes,
					["error"] = result.Error ?? ""
				});
			}
			catch (Exception ex)
			{
				Send(new JObject
				{
					["type"] = "cfb27EAmitmImportResult",
					["ok"] = false,
					["error"] = ex.Message
				});
			}
		});
	}

	private CFB27DiscoveryCapture GetCFB27Capture()
	{
		return m_cfb27Capture ??= new CFB27DiscoveryCapture(s_httpClient, GetAppdataDir);
	}

	private CFB27EAmitmImport GetCFB27EAmitmImport()
	{
		return m_cfb27EAmitmImport ??= new CFB27EAmitmImport(GetAppdataDir);
	}

	private CFB27EndpointExperiment GetCFB27EndpointExperiment()
	{
		return m_cfb27EndpointExperiment ??= new CFB27EndpointExperiment(GetAppdataDir);
	}

	private List<CFB27CaptureInstance> GetCFB27CaptureInstances()
	{
		lock (m_instanceLock)
		{
			return m_instances.Values
				.Where(i => i.Game == PVZGame.CFB27.ToString())
				.Select(i => new CFB27CaptureInstance(
					i.Pid,
					i.Game,
					i.IsServer,
					i.ClientGamePort,
					i.ServerGamePort,
					i.StartTime.ToString("o"),
					SafeHasExited(i.Process),
					i.Process.StartInfo.Arguments,
					i.Process.StartInfo.Environment.TryGetValue("CYPRESS_MASTER_URL", out var masterUrl) ? masterUrl : "",
					i.Process.StartInfo.Environment.TryGetValue("CYPRESS_CFB27_DYNASTY_URL", out var dynastyUrl) ? dynastyUrl : "",
					i.Process.StartInfo.Environment.TryGetValue("CYPRESS_SIDE_CHANNEL_PORT", out var sideChannel) && int.TryParse(sideChannel, out var sidePort) ? sidePort : 0,
					i.Process.StartInfo.Environment.TryGetValue("CYPRESS_CFB27_DYNASTY_PROFILE", out var profile) ? profile : "default"))
				.ToList();
		}
	}

	private static bool SafeHasExited(Process p)
	{
		try { return p.HasExited; } catch { return true; }
	}

	private void RecordCFB27Event(string line)
	{
		lock (m_cfb27RecentEventsLock)
		{
			m_cfb27RecentEvents.Enqueue($"{DateTime.Now:O} {line}");
			while (m_cfb27RecentEvents.Count > 500)
				m_cfb27RecentEvents.Dequeue();
		}
	}

	private List<string> GetCFB27RecentEvents()
	{
		lock (m_cfb27RecentEventsLock)
			return m_cfb27RecentEvents.ToList();
	}

	private string FindCypressRoot()
	{
		var dir = new DirectoryInfo(AppContext.BaseDirectory);
		while (dir != null)
		{
			if (Directory.Exists(Path.Combine(dir.FullName, "tools", "cypress-servers")) &&
				Directory.Exists(Path.Combine(dir.FullName, "Launcher")))
				return dir.FullName;
			dir = dir.Parent;
		}
		return Path.GetFullPath(Path.Combine(AppContext.BaseDirectory, "..", ".."));
	}

	private static string FindToolExe(string servicesDir, string buildDir, string exeName)
	{
		string built = Path.Combine(buildDir, exeName);
		if (File.Exists(built))
			return built;
		string local = Path.Combine(servicesDir, exeName);
		if (File.Exists(local))
			return local;
		throw new FileNotFoundException($"Could not find {exeName}. Run tools/cypress-servers/build.ps1 first.", exeName);
	}

	private async Task EnsureDiagnosticProcessAsync(
		string name,
		string exe,
		IEnumerable<string> args,
		string workingDir,
		string healthUrl,
		int startupTimeoutSeconds = 5,
		bool restartIfHealthy = false)
	{
		if (await IsDiagnosticServiceHealthyAsync(healthUrl))
		{
			if (restartIfHealthy)
			{
				LogCFB27PrivateStart($"{name}: healthy existing service found at {healthUrl}; restarting");
				StopDiagnosticProcesses(name);
				await Task.Delay(500);
			}
			else
			{
				RecordCFB27Event($"diagnostics: reused existing {name} service at {healthUrl}");
				LogCFB27PrivateStart($"{name}: reused existing healthy service at {healthUrl}");
				return;
			}
		}

		if (name.Equals("cfb27blaze", StringComparison.OrdinalIgnoreCase))
		{
			string pendingExe = Path.Combine(Path.GetDirectoryName(exe)!, "cfb27blaze.next.exe");
			if (File.Exists(pendingExe))
			{
				File.Copy(pendingExe, exe, overwrite: true);
				File.Delete(pendingExe);
				LogCFB27PrivateStart($"{name}: promoted tested pending update to {exe}");
			}
		}

		LogCFB27PrivateStart($"{name}: starting {exe}");
		LogCFB27PrivateStart($"{name}: args={string.Join(" ", args.Select(a => a.Contains(' ') ? "\"" + a + "\"" : a))}");
		LogCFB27PrivateStart($"{name}: workingDir={workingDir}");
		var proc = StartDiagnosticProcess(exe, args, workingDir);
		RecordCFB27Event($"diagnostics: started {name} pid={proc.Id}");
		LogCFB27PrivateStart($"{name}: started pid={proc.Id}");

		DateTime deadline = DateTime.UtcNow.AddSeconds(startupTimeoutSeconds);
		while (DateTime.UtcNow < deadline)
		{
			if (await IsDiagnosticServiceHealthyAsync(healthUrl))
			{
				LogCFB27PrivateStart($"{name}: healthy at {healthUrl}");
				return;
			}

			if (proc.HasExited)
			{
				string stdout = await ReadProcessStreamSafeAsync(proc.StandardOutput);
				string stderr = await ReadProcessStreamSafeAsync(proc.StandardError);
				LogCFB27PrivateStart($"{name}: exited code={proc.ExitCode}");
				if (!string.IsNullOrWhiteSpace(stderr))
					LogCFB27PrivateStart($"{name}: stderr={stderr}");
				if (!string.IsNullOrWhiteSpace(stdout))
					LogCFB27PrivateStart($"{name}: stdout={stdout}");
				throw new InvalidOperationException($"{name} diagnostics service exited early with code {proc.ExitCode}. {stderr} {stdout}".Trim());
			}

			await Task.Delay(250);
		}

		LogCFB27PrivateStart($"{name}: timeout waiting for {healthUrl}");
		throw new TimeoutException($"{name} diagnostics service did not become healthy at {healthUrl}.");
	}

	private void StopDiagnosticProcesses(string name)
	{
		foreach (var proc in Process.GetProcessesByName(name))
		{
			try
			{
				LogCFB27PrivateStart($"{name}: stopping existing pid={proc.Id}");
				proc.Kill(entireProcessTree: true);
				proc.WaitForExit(5000);
				LogCFB27PrivateStart($"{name}: stopped pid={proc.Id}");
			}
			catch (Exception ex)
			{
				LogCFB27PrivateStart($"{name}: failed to stop pid={proc.Id}: {ex.Message}");
			}
			finally
			{
				proc.Dispose();
			}
		}
	}

	private static async Task<bool> IsDiagnosticServiceHealthyAsync(string url)
	{
		try
		{
			using var resp = await s_httpClient.GetAsync(url);
			return resp.IsSuccessStatusCode;
		}
		catch
		{
			return false;
		}
	}

	private static async Task<string> ReadProcessStreamSafeAsync(StreamReader reader)
	{
		try
		{
			if (reader.EndOfStream)
				return "";
			return await reader.ReadToEndAsync();
		}
		catch
		{
			return "";
		}
	}

	private Process StartDiagnosticProcess(string exe, IEnumerable<string> args, string workingDir)
	{
		var psi = new ProcessStartInfo
		{
			FileName = exe,
			WorkingDirectory = workingDir,
			UseShellExecute = false,
			CreateNoWindow = true,
			RedirectStandardOutput = true,
			RedirectStandardError = true
		};
		foreach (string arg in args)
			psi.ArgumentList.Add(arg);
		var proc = Process.Start(psi);
		if (proc == null)
			throw new InvalidOperationException("Failed to start " + exe);
		lock (m_diagnosticProcesses)
			m_diagnosticProcesses.Add(proc);
		return proc;
	}
}
