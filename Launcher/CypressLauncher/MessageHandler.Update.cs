#nullable enable
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Net.Http;
using System.Reflection;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;

namespace CypressLauncher;

public partial class MessageHandler
{
	private static readonly string s_updateSavedataKey = "Updates";
	private static readonly string[] s_serverUpdateGames = { "GW1", "GW2", "BFN" };

	private sealed class UpdateChannel
	{
		public string Id;
		public string DisplayName;
		public string RepoOwner;
		public string RepoName;
		public string LocalVersion;
		public string? AssetPattern;

		public string? LatestTag;
		public string? LatestBody;
		public string? AssetUrl;
		public long AssetSize;

		public UpdateChannel(string id, string displayName, string repoOwner, string repoName, string localVersion, string? assetPattern = null)
		{
			Id = id;
			DisplayName = displayName;
			RepoOwner = repoOwner;
			RepoName = repoName;
			LocalVersion = localVersion;
			AssetPattern = assetPattern;
		}
	}

	private UpdateChannel[] GetUpdateChannels()
	{
		string launcherVersion = GetLauncherVersion();
		string serverVersion = GetSavedServerDllVersion(launcherVersion);

		return new[]
		{
			new UpdateChannel("launcher", "Cypress Launcher", "PvZ-Cypress", "Cypress", launcherVersion),
			new UpdateChannel("server", "Cypress Server", "PvZ-Cypress", "Cypress", serverVersion)
		};
	}

	private static string GetLauncherVersion()
	{
		var assembly = Assembly.GetExecutingAssembly();
		string? infoVersion = assembly.GetCustomAttribute<AssemblyInformationalVersionAttribute>()?.InformationalVersion;
		if (!string.IsNullOrWhiteSpace(infoVersion))
			return NormalizeVersion(infoVersion.Split('+')[0]);
		return assembly.GetName().Version?.ToString(3) ?? "0.0.0";
	}

	private string GetSavedServerDllVersion(string bundledVersion)
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			if (File.Exists(filePath))
			{
				var root = JObject.Parse(File.ReadAllText(filePath));
				var updates = root[s_updateSavedataKey] as JObject;
				string? savedVersion = (string?)updates?["server_dll_version"];
				if (!string.IsNullOrWhiteSpace(savedVersion))
					return NormalizeVersion(savedVersion);
			}

			return HasBundledServerDlls() ? bundledVersion : "0.0.0";
		}
		catch { return HasBundledServerDlls() ? bundledVersion : "0.0.0"; }
	}

	private static bool HasBundledServerDlls()
	{
		return s_serverUpdateGames.All(game =>
			File.Exists(Path.Combine(AppContext.BaseDirectory, $"cypress_{game}.dll")));
	}

	private void SaveServerDllVersion(string version)
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			JObject root = new JObject();
			if (File.Exists(filePath))
				root = JObject.Parse(File.ReadAllText(filePath));
			var updates = root[s_updateSavedataKey] as JObject ?? new JObject();
			updates["server_dll_version"] = NormalizeVersion(version);
			root[s_updateSavedataKey] = updates;
			File.WriteAllText(filePath, root.ToString());
		}
		catch { }
	}

	private static bool IsNewerVersion(string local, string remote)
	{
		if (!TryParseUpdateVersion(local, out var localVer) || !TryParseUpdateVersion(remote, out var remoteVer))
			return false;

		return remoteVer != null && localVer != null && remoteVer > localVer;
	}

	private static string NormalizeVersion(string version)
	{
		version = version.Trim().TrimStart('v', 'V');
		if (version.Length == 0) return "0.0.0";

		var match = Regex.Match(version, @"^\d+(?:\.\d+){0,3}");
		return match.Success ? match.Value : version;
	}

	private static bool TryParseUpdateVersion(string value, out Version? version)
	{
		string normalized = NormalizeVersion(value);
		if (normalized.Count(c => c == '.') == 0)
			normalized += ".0";
		return Version.TryParse(normalized, out version);
	}

	private void OnCheckUpdates()
	{
		Task.Run(async () =>
		{
			var channels = GetUpdateChannels();
			var updates = new JArray();
			var releaseCache = new Dictionary<string, JObject>();

			foreach (var ch in channels)
			{
				try
				{
					string repoKey = $"{ch.RepoOwner}/{ch.RepoName}";
					if (!releaseCache.TryGetValue(repoKey, out var json))
					{
						var request = new HttpRequestMessage(HttpMethod.Get,
							$"https://api.github.com/repos/{ch.RepoOwner}/{ch.RepoName}/releases/latest");
						request.Headers.Add("User-Agent", "CypressLauncher");
						request.Headers.Add("Accept", "application/vnd.github+json");

						var resp = await s_httpClient.SendAsync(request);
						if (!resp.IsSuccessStatusCode) continue;

						json = JObject.Parse(await resp.Content.ReadAsStringAsync());
						releaseCache[repoKey] = json;
					}
					string? tag = (string?)json["tag_name"];
					if (tag == null) continue;

					if (!IsNewerVersion(ch.LocalVersion, tag)) continue;

					ch.LatestTag = tag;
					ch.LatestBody = (string?)json["body"] ?? "";

					// find zip asset
					var assets = json["assets"] as JArray;
					if (assets != null)
					{
						foreach (var asset in assets)
						{
							string? name = (string?)asset["name"];
							if (name == null) continue;
							if (!name.EndsWith(".zip", StringComparison.OrdinalIgnoreCase)) continue;

							// if pattern set, match it; otherwise just take first zip
							if (ch.AssetPattern != null && !name.Contains(ch.AssetPattern, StringComparison.OrdinalIgnoreCase))
								continue;

							ch.AssetUrl = (string?)asset["browser_download_url"];
							ch.AssetSize = (long)(asset["size"] ?? 0);
							break;
						}
					}

					if (ch.AssetUrl == null) continue;

					updates.Add(new JObject
					{
						["channel"] = ch.Id,
						["name"] = ch.DisplayName,
						["currentVersion"] = ch.LocalVersion,
						["latestVersion"] = ch.LatestTag,
						["releaseNotes"] = ch.LatestBody,
						["assetUrl"] = ch.AssetUrl,
						["assetSize"] = ch.AssetSize
					});
				}
				catch { }
			}

			if (updates.OfType<JObject>().Any(u => (string?)u["channel"] == "launcher"))
			{
				foreach (var update in updates.OfType<JObject>().Where(u => (string?)u["channel"] == "server").ToList())
					update.Remove();
			}

			Send(new JObject { ["type"] = "updateCheckResult", ["updates"] = updates });
		});
	}

	private void OnStartUpdate(JObject msg)
	{
		string channel = (string?)msg["channel"] ?? "";
		string assetUrl = (string?)msg["assetUrl"] ?? "";
		string latestVersion = NormalizeVersion((string?)msg["latestVersion"] ?? "");

		if (string.IsNullOrEmpty(channel) || string.IsNullOrEmpty(assetUrl))
		{
			Send(new JObject { ["type"] = "updateError", ["channel"] = channel, ["error"] = "Missing update info" });
			return;
		}
		if (channel != "server" && channel != "launcher")
		{
			Send(new JObject { ["type"] = "updateError", ["channel"] = channel, ["error"] = "Unknown update channel" });
			return;
		}

		if (!IsAllowedAssetUrl(assetUrl))
		{
			Send(new JObject { ["type"] = "updateError", ["channel"] = channel, ["error"] = "update url is not from an allowed source" });
			return;
		}

		string currentVersion = channel == "launcher"
			? GetLauncherVersion()
			: GetSavedServerDllVersion(GetLauncherVersion());
		if (!IsNewerVersion(currentVersion, latestVersion))
		{
			Send(new JObject { ["type"] = "updateError", ["channel"] = channel, ["error"] = "update version is not newer than the installed version" });
			return;
		}

		Task.Run(async () =>
		{
			try
			{
				string tempDir = Path.Combine(Path.GetTempPath(), "cypress-update", channel);
				ResetUpdateDirectory(tempDir);

				string zipPath = Path.Combine(tempDir, "update.zip");

				// download with progress
				var request = new HttpRequestMessage(HttpMethod.Get, assetUrl);
				request.Headers.Add("User-Agent", "CypressLauncher");
				var resp = await s_httpClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead);
				resp.EnsureSuccessStatusCode();

				long totalBytes = resp.Content.Headers.ContentLength ?? -1;
				long downloaded = 0;
				int lastPercent = -1;

				using (var contentStream = await resp.Content.ReadAsStreamAsync())
				using (var fileStream = new FileStream(zipPath, FileMode.Create, FileAccess.Write, FileShare.None, 8192, true))
				{
					var buffer = new byte[65536];
					int bytesRead;
					while ((bytesRead = await contentStream.ReadAsync(buffer, 0, buffer.Length)) > 0)
					{
						await fileStream.WriteAsync(buffer, 0, bytesRead);
						downloaded += bytesRead;

						if (totalBytes > 0)
						{
							int percent = (int)(downloaded * 100 / totalBytes);
							if (percent != lastPercent)
							{
								lastPercent = percent;
								Send(new JObject { ["type"] = "updateProgress", ["channel"] = channel, ["percent"] = percent });
							}
						}
					}
				}
				if (lastPercent != 100)
					Send(new JObject { ["type"] = "updateProgress", ["channel"] = channel, ["percent"] = 100 });

				string extractDir = Path.Combine(tempDir, "extracted");
				ZipFile.ExtractToDirectory(zipPath, extractDir, true);

				var topDirs = Directory.GetDirectories(extractDir);
				var topFiles = Directory.GetFiles(extractDir);
				if (topDirs.Length == 1 && topFiles.Length == 0)
					extractDir = topDirs[0];

				if (channel == "server")
				{
					ApplyServerUpdate(extractDir, latestVersion);
				}
				else if (channel == "launcher")
				{
					ApplyLauncherUpdate(extractDir, latestVersion);
				}

				Send(new JObject { ["type"] = "updateComplete", ["channel"] = channel, ["version"] = latestVersion });
			}
			catch (Exception ex)
			{
				Send(new JObject { ["type"] = "updateError", ["channel"] = channel, ["error"] = ex.Message });
			}
		});
	}

	private static void ResetUpdateDirectory(string dir)
	{
		for (int i = 0; i < 5; i++)
		{
			try
			{
				if (Directory.Exists(dir)) Directory.Delete(dir, true);
				Directory.CreateDirectory(dir);
				return;
			}
			catch when (i < 4)
			{
				Thread.Sleep(250);
			}
		}

		if (Directory.Exists(dir)) Directory.Delete(dir, true);
		Directory.CreateDirectory(dir);
	}

	private void ApplyServerUpdate(string extractDir, string version)
	{
		string installDir = AppContext.BaseDirectory;
		int updated = 0;

		foreach (string game in s_serverUpdateGames)
		{
			string dllName = $"cypress_{game}.dll";
			string srcPath = Path.Combine(extractDir, dllName);
			if (!File.Exists(srcPath))
			{
				var found = Directory.GetFiles(extractDir, dllName, SearchOption.AllDirectories).FirstOrDefault();
				if (found == null) continue;
				srcPath = found;
			}

			File.Copy(srcPath, Path.Combine(installDir, dllName), overwrite: true);
			updated++;
		}

		if (updated == 0)
			throw new InvalidDataException("Update package contained no server DLLs");

		SaveServerDllVersion(version);
		SendStatus($"Server DLLs updated to {version} ({updated}/{s_serverUpdateGames.Length})", "info");
	}

	private void ApplyLauncherUpdate(string extractDir, string version)
	{
		string installDir = AppContext.BaseDirectory;
		string tempDir = Path.Combine(Path.GetTempPath(), "cypress-update", "launcher");
		string exePath = Path.Combine(extractDir, "CypressLauncher.exe");
		if (!File.Exists(exePath))
		{
			string? foundExe = Directory.GetFiles(extractDir, "CypressLauncher.exe", SearchOption.AllDirectories).FirstOrDefault();
			if (foundExe == null)
				throw new InvalidDataException("Update package missing CypressLauncher.exe");
			extractDir = Path.GetDirectoryName(foundExe) ?? extractDir;
		}

		try { ApplyServerUpdate(extractDir, version); } catch { }

		string scriptPath = Path.Combine(tempDir, "apply.ps1");
		string logPath = Path.Combine(tempDir, "apply.log");
		int pid = Environment.ProcessId;

		string script = $@"
$ErrorActionPreference = 'Stop'
$log = {PsQuote(logPath)}
function Write-UpdateLog([string]$message) {{
    Add-Content -LiteralPath $log -Value (""{{0}} {{1}}"" -f (Get-Date -Format o), $message)
}}
try {{
    Write-UpdateLog 'waiting for launcher'
    $proc = Get-Process -Id {pid} -ErrorAction SilentlyContinue
    if ($proc) {{
        if (-not $proc.WaitForExit(60000)) {{
            Stop-Process -Id {pid} -Force -ErrorAction SilentlyContinue
            Start-Sleep -Milliseconds 1000
        }}
    }}
    $source = {PsQuote(extractDir)}
    $dest = {PsQuote(installDir)}
    Write-UpdateLog 'copying update files'
    $copy = Start-Process -FilePath 'robocopy.exe' -ArgumentList @($source, $dest, '/E', '/R:30', '/W:1', '/NFL', '/NDL', '/NJH', '/NJS', '/NP') -Wait -PassThru -WindowStyle Hidden
    if ($copy.ExitCode -ge 8) {{ throw ""robocopy failed with exit code $($copy.ExitCode)"" }}
    Write-UpdateLog 'starting launcher'
    Start-Process -FilePath {PsQuote(Path.Combine(installDir, "CypressLauncher.exe"))} -WorkingDirectory $dest
}} catch {{
    Add-Content -LiteralPath $log -Value (""{{0}} failed: {{1}}"" -f (Get-Date -Format o), $_.Exception.Message)
    try {{ Start-Process -FilePath {PsQuote(Path.Combine(installDir, "CypressLauncher.exe"))} -WorkingDirectory $dest }} catch {{}}
}}
";
		File.WriteAllText(scriptPath, script, Encoding.UTF8);

		var psi = new ProcessStartInfo
		{
			FileName = "powershell.exe",
			Arguments = $"-ExecutionPolicy Bypass -WindowStyle Hidden -File \"{scriptPath}\"",
			UseShellExecute = true,
			CreateNoWindow = true,
			WindowStyle = ProcessWindowStyle.Hidden
		};
		Process.Start(psi);

		SendStatus("Restarting to apply update...", "info");
		Environment.Exit(0);
	}

	private static bool IsAllowedAssetUrl(string url)
	{
		if (!Uri.TryCreate(url, UriKind.Absolute, out var uri)) return false;
		if (uri.Scheme != Uri.UriSchemeHttps) return false;
		if (!uri.Host.Equals("github.com", StringComparison.OrdinalIgnoreCase)) return false;
		if (!uri.AbsolutePath.StartsWith("/PvZ-Cypress/Cypress/releases/download/", StringComparison.OrdinalIgnoreCase)) return false;
		return true;
	}

	private static string PsQuote(string value)
	{
		return "'" + value.Replace("'", "''") + "'";
	}
}
