#nullable enable
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;

namespace CypressLauncher;

public partial class MessageHandler
{
	private void OnJoin(JObject msg)
	{
		long now = Environment.TickCount64;
		if (now - m_lastLaunchTicks < 3000) return;
		m_lastLaunchTicks = now;
		string username = ((string?)msg["username"]) ?? "";
		string serverIP = ((string?)msg["serverIP"]) ?? "";
		int gamePort = (int)(msg["gamePort"] ?? 0);
		string joinConnectionMode = ((string?)msg["joinConnectionMode"]) ?? "Direct";
		string joinRelayAddress = ((string?)msg["joinRelayAddress"]) ?? "";
		string joinRelayKey = ((string?)msg["joinRelayKey"]) ?? "";
		string serverPassword = ((string?)msg["serverPassword"]) ?? "";
		string fovStr = ((string?)msg["fov"]) ?? "";
		string additionalArgs = ((string?)msg["additionalArgs"]) ?? "";
		bool useMods = (bool)(msg["useMods"] ?? false);
		string modPack = ((string?)msg["modPack"]) ?? "";
		bool useRelay = string.Equals(joinConnectionMode, "Relay", StringComparison.OrdinalIgnoreCase);
		string effectiveServerIP = useRelay ? string.Empty : serverIP;

		if (string.IsNullOrWhiteSpace(m_gameDirectory))
		{
			SendStatus("Game directory not set.", "error");
			return;
		}
		if (!File.Exists(GetServerDLLName()))
		{
			SendStatus("Server DLL not found. Verify that " + GetServerDLLName() + " is in the launcher's folder.", "error");
			return;
		}
		if (!ConfigureProxyEnvironment(useRelay, joinRelayAddress, joinRelayKey, out string relayHost))
			return;
		if (useRelay && string.IsNullOrWhiteSpace(effectiveServerIP))
			effectiveServerIP = relayHost;
		if (string.IsNullOrEmpty(effectiveServerIP))
		{
			SendStatus("Must enter a server address.", "error");
			return;
		}
		if (string.IsNullOrWhiteSpace(username))
		{
			SendStatus("Username cannot be empty.", "error");
			return;
		}
		if (username.Length < 3)
		{
			SendStatus("Username must be at least 3 characters.", "error");
			return;
		}
		if (username.Length > 32)
		{
			SendStatus("Username cannot be longer than 32 characters.", "error");
			return;
		}

		SaveCurrentFormData(msg);

		string exeName = s_gameToExecutableName[m_selectedGame];
		bool failed = false;
		if (GameRequiresPatchedExe(Path.Combine(m_gameDirectory, exeName), ref failed) && !failed)
		{
			string patchedExeName = s_gameToPatchedExecutableName[m_selectedGame];
			if (!PatchManager.EnsurePatched(m_selectedGame, m_gameDirectory, exeName, patchedExeName, SendStatus))
				return;
			exeName = patchedExeName;
		}
		if (failed) return;

		Environment.SetEnvironmentVariable("EARtPLaunchCode", GetRtPLaunchCode().ToString());

		string contentId = "0";
		if (m_selectedGame == PVZGame.GW1)
			contentId = "1011216";
		else if (m_selectedGame == PVZGame.GW2)
			contentId = "1026482";
		else if (m_selectedGame == PVZGame.BFN)
			contentId = "1036445";

		Environment.SetEnvironmentVariable("ContentId", contentId);

		bool useMod = useMods && !string.IsNullOrEmpty(modPack);
		Environment.SetEnvironmentVariable("GAME_DATA_DIR", useMod ? Path.Combine(m_gameDirectory, "ModData", modPack) : null);

		// use gamePort from serverInfo if provided, so clients connect to the right port when multiple servers run
		string serverIPOnly = effectiveServerIP;
		int colonIdx = effectiveServerIP.LastIndexOf(':');
		if (colonIdx > 0 && int.TryParse(effectiveServerIP.Substring(colonIdx + 1), out int addrPort))
		{
			serverIPOnly = effectiveServerIP.Substring(0, colonIdx);
			if (gamePort <= 0) gamePort = addrPort; // use port from typed address if no gamePort from serverInfo
		}
		string serverIPWithPort = gamePort > 0 ? serverIPOnly + ":" + gamePort : serverIPOnly;

		// use nickname as in-game name if set, otherwise use the username field
		string inGameName = !string.IsNullOrWhiteSpace(m_identityNickname) ? m_identityNickname : username;
		string launchArgs = $"-playerName \"{inGameName}\" -console -Client.ServerIp {serverIPWithPort} -allowMultipleInstances -RenderDevice.IntelMinDriverVersion 0.0";
		if (!string.IsNullOrWhiteSpace(serverPassword))
			launchArgs += " -password " + serverPassword;
		if (s_specialLaunchArgsForGame.TryGetValue(m_selectedGame, out string? specialArgs))
			launchArgs += " " + specialArgs;
		if (useMod && m_selectedGame == PVZGame.BFN)
			launchArgs += " -datapath \"" + Path.Combine(m_gameDirectory, "ModData", modPack) + "\"";
		if (!string.IsNullOrWhiteSpace(fovStr) && double.TryParse(fovStr, out double fovValue))
			launchArgs += " -Render.FovMultiplier " + (fovValue / 70.0).ToString();
		if (!string.IsNullOrWhiteSpace(additionalArgs))
			launchArgs += " " + additionalArgs;

		Environment.SetEnvironmentVariable("GW_LAUNCH_ARGS", launchArgs);
		if (m_identityJwt == null) LoadIdentityFromDisk();
		m_identityKey ??= LoadOrCreateIdentityKey();
		Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_JWT", m_identityJwt);
		Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_KEY", GetIdentityPrivateKeyHex());
		if (!CopyServerDLL()) return;

		LaunchGame(exeName, launchArgs);
	}

	private void OnStartServer(JObject msg)
	{
		long now = Environment.TickCount64;
		if (now - m_lastLaunchTicks < 3000) return;
		m_lastLaunchTicks = now;
		string deviceIP = ((string?)msg["deviceIP"]) ?? "";
		string hostConnectionMode = ((string?)msg["hostConnectionMode"]) ?? "Direct";
		string hostRelayAddress = ((string?)msg["hostRelayAddress"]) ?? "";
		string hostRelayKey = ((string?)msg["hostRelayKey"]) ?? "";

		// default relay address when mode is Relay but address is empty
		if (string.Equals(hostConnectionMode, "Relay", StringComparison.OrdinalIgnoreCase)
			&& string.IsNullOrWhiteSpace(hostRelayAddress))
		{
			hostRelayAddress = LauncherConfig.RelayNA;
		}
		string level = ((string?)msg["level"]) ?? "";
		string inclusion = ((string?)msg["inclusion"]) ?? "";
		string startPoint = ((string?)msg["startPoint"]) ?? "";
		string dedicatedPassword = ((string?)msg["dedicatedPassword"]) ?? "";
		string playerCount = ((string?)msg["playerCount"]) ?? "";
		bool usePlaylist = (bool)(msg["usePlaylist"] ?? false);
		string playlist = ((string?)msg["playlist"]) ?? "";
		bool allowAIBackfill = (bool)(msg["allowAIBackfill"] ?? false);
		string serverAdditionalArgs = ((string?)msg["serverAdditionalArgs"]) ?? "";
		bool useMods = (bool)(msg["useMods"] ?? false);
		string modPack = ((string?)msg["modPack"]) ?? "";
		string loadScreenGameMode = ((string?)msg["loadScreenGameMode"]) ?? "";
		string loadScreenLevelName = ((string?)msg["loadScreenLevelName"]) ?? "";
		string loadScreenLevelDescription = ((string?)msg["loadScreenLevelDescription"]) ?? "";
		string loadScreenUIAssetPath = ((string?)msg["loadScreenUIAssetPath"]) ?? "";

		if (string.IsNullOrWhiteSpace(m_gameDirectory))
		{
			SendStatus("Game directory not set.", "error");
			return;
		}
		if (!File.Exists(GetServerDLLPath()))
		{
			SendStatus("Server DLL not found. Verify that " + GetServerDLLName() + " is in the launcher's folder.", "error");
			return;
		}
		if (!ConfigureProxyEnvironment(string.Equals(hostConnectionMode, "Relay", StringComparison.OrdinalIgnoreCase), hostRelayAddress, hostRelayKey, out _))
			return;

		if (string.IsNullOrWhiteSpace(deviceIP))
			deviceIP = TryGetPreferredDeviceIp();
		if (string.IsNullOrEmpty(deviceIP))
		{
			SendStatus("Could not determine a local IPv4 automatically. Enter a bind address manually.", "error");
			return;
		}
		if (!System.Net.IPAddress.TryParse(deviceIP, out var parsedIp) || parsedIp.AddressFamily != System.Net.Sockets.AddressFamily.InterNetwork)
		{
			SendStatus("Device IP must be a valid IPv4 address.", "error");
			return;
		}
		if (string.IsNullOrWhiteSpace(level))
		{
			SendStatus("Level not set.", "error");
			return;
		}
		if (string.IsNullOrWhiteSpace(inclusion))
		{
			SendStatus("Level's Inclusion not set.", "error");
			return;
		}

		SaveCurrentFormData(msg);

		string exeName = s_gameToExecutableName[m_selectedGame];
		bool failed = false;
		if (GameRequiresPatchedExe(Path.Combine(m_gameDirectory, exeName), ref failed) && !failed)
		{
			string patchedExeName = s_gameToPatchedExecutableName[m_selectedGame];
			if (!PatchManager.EnsurePatched(m_selectedGame, m_gameDirectory, exeName, patchedExeName, SendStatus))
				return;
			exeName = patchedExeName;
		}
		if (failed) return;

		Environment.SetEnvironmentVariable("EARtPLaunchCode", GetRtPLaunchCode().ToString());

		string contentId = "0";
		if (m_selectedGame == PVZGame.GW1)
			contentId = "1011216";
		else if (m_selectedGame == PVZGame.GW2)
			contentId = "1026482";
		else if (m_selectedGame == PVZGame.BFN)
			contentId = "1036445";

		Environment.SetEnvironmentVariable("ContentId", contentId);

		bool useMod = useMods && !string.IsNullOrEmpty(modPack);
		Environment.SetEnvironmentVariable("GAME_DATA_DIR", useMod ? Path.Combine(m_gameDirectory, "ModData", modPack) : null);
		bool playlistFlag = usePlaylist && !string.IsNullOrEmpty(playlist);

		string launchArgs;
		if (m_selectedGame < PVZGame.BFN)
		{
			launchArgs = $"-server -level {level} -listen {deviceIP} -inclusion {inclusion} -allowMultipleInstances -Network.ServerAddress {deviceIP}";
			if (!string.IsNullOrWhiteSpace(loadScreenGameMode))
				launchArgs += " -loadScreenGameMode " + loadScreenGameMode;
			if (!string.IsNullOrWhiteSpace(loadScreenLevelName))
				launchArgs += " -loadScreenLevelName " + loadScreenLevelName;
			if (!string.IsNullOrWhiteSpace(loadScreenLevelDescription))
				launchArgs += " -loadScreenLevelDescription " + loadScreenLevelDescription;
			if (!string.IsNullOrWhiteSpace(loadScreenUIAssetPath))
				launchArgs += " -loadScreenUIAssetPath " + loadScreenUIAssetPath;
			if (!string.IsNullOrWhiteSpace(dedicatedPassword))
				launchArgs += " -Server.ServerPassword " + dedicatedPassword;
			if (playlistFlag)
				launchArgs += " -usePlaylist -playlistFilename \"" + Path.Combine(m_gameDirectory, "Playlists", playlist) + "\"";
			if (s_serverLaunchArgsForGame.TryGetValue(m_selectedGame, out string? sArgs))
				launchArgs += " " + sArgs;
			if (!string.IsNullOrWhiteSpace(serverAdditionalArgs))
				launchArgs += " " + serverAdditionalArgs;
			if (!string.IsNullOrWhiteSpace(playerCount))
				launchArgs += " -Network.MaxClientCount " + playerCount;
		}
		else
		{
			launchArgs = $"-server -listen {deviceIP} -dsub {level} -inclusion {inclusion} -startpoint {startPoint} -allowMultipleInstances -enableServerLog -Network.ServerAddress {deviceIP}";
			if (!string.IsNullOrWhiteSpace(dedicatedPassword))
				launchArgs += " -Server.ServerPassword " + dedicatedPassword;
			if (playlistFlag)
				launchArgs += " -usePlaylist -playlistFilename \"" + Path.Combine(m_gameDirectory, "Playlists", playlist) + "\"";
			if (useMod)
				launchArgs += " -datapath \"" + Path.Combine(m_gameDirectory, "ModData", modPack) + "\"";
			if (!allowAIBackfill)
				launchArgs += " -GameMode.BackfillMpWithAI false";
			if (s_serverLaunchArgsForGame.TryGetValue(m_selectedGame, out string? sArgs))
				launchArgs += " " + sArgs;
			if (!string.IsNullOrWhiteSpace(serverAdditionalArgs))
				launchArgs += " " + serverAdditionalArgs;
			if (!string.IsNullOrWhiteSpace(playerCount))
				launchArgs += " -Network.MaxClientCount " + playerCount + " -NetObjectSystem.MaxServerConnectionCount " + playerCount + " -Online.DirtySockMaxConnectionCount " + playerCount;
		}

		Environment.SetEnvironmentVariable("GW_LAUNCH_ARGS", launchArgs);
		if (m_identityJwt == null) LoadIdentityFromDisk();
		m_identityKey ??= LoadOrCreateIdentityKey();
		Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_JWT", m_identityJwt);
		Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_KEY", GetIdentityPrivateKeyHex());
		if (!CopyServerDLL()) return;

		LaunchGame(exeName, launchArgs, isServer: true, level: level, msg: msg);
	}

	private bool CopyServerDLL()
	{
		string srcPath = GetServerDLLPath();
		string destPath = Path.Combine(m_gameDirectory, s_destDLLName);

		if (PatchManager.SameFileContentsSafe(srcPath, destPath))
			return true;

		try
		{
			File.Copy(srcPath, destPath, overwrite: true);
			return true;
		}
		catch (IOException) when (File.Exists(destPath))
		{
			SendStatus("failed to update dinput8.dll because the game is still using an older one.", "error");
			return false;
		}
		catch (UnauthorizedAccessException)
		{
			return TryCopyDllElevated(srcPath, destPath);
		}
		catch (Exception ex)
		{
			SendStatus("Failed to copy DLL: " + ex.Message, "error");
			return false;
		}
	}

	private bool TryCopyDllElevated(string src, string dest)
	{
		if (PatchManager.IsWine())
		{
			SendStatus("Failed to copy DLL: access denied. Try moving the game outside of Program Files.", "error");
			return false;
		}
		try
		{
			SendStatus("Copying DLL requires administrator permission. Please approve the prompt.", "info");
			var startInfo = new ProcessStartInfo
			{
				FileName = "cmd.exe",
				Arguments = $"/c copy /y \"{src}\" \"{dest}\"",
				Verb = "runas",
				UseShellExecute = true,
				WindowStyle = ProcessWindowStyle.Hidden
			};
			var process = System.Diagnostics.Process.Start(startInfo);
			process?.WaitForExit();
			if (process?.ExitCode == 0) return true;
			SendStatus("Failed to copy DLL (elevated, code: " + process?.ExitCode.ToString("X") + ")", "error");
			return false;
		}
		catch (Exception ex)
		{
			SendStatus("Failed to copy DLL: " + ex.Message, "error");
			return false;
		}
	}

	private int FindFreeSideChannelPort()
	{
		var usedPorts = new HashSet<int>();
		lock (m_instanceLock)
		{
			foreach (var inst in m_instances.Values)
			{
				if (inst.IsServer)
				{
					string portFile = Path.Combine(Path.GetTempPath(), $"cypress_{inst.Pid}.port");
					try
					{
						if (File.Exists(portFile))
						{
							var info = JObject.Parse(File.ReadAllText(portFile));
							int p = (int)(info["port"] ?? 0);
							if (p > 0) usedPorts.Add(p);
						}
					}
					catch { }
				}
			}
		}
		for (int port = 14638; port < 14700; port++)
		{
			if (!usedPorts.Contains(port))
			{
				try
				{
					using var sock = new System.Net.Sockets.TcpListener(System.Net.IPAddress.Loopback, port);
					sock.Start();
					sock.Stop();
					return port;
				}
				catch { }
			}
		}
		return 14638;
	}

	private static string SanitizeModpackUrl(string? url)
	{
		if (string.IsNullOrWhiteSpace(url)) return "";
		url = url.Trim();
		if (url.StartsWith("http://", StringComparison.OrdinalIgnoreCase) ||
			url.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
			return url;
		return "";
	}

	private int FindFreeClientGamePort()
	{
		var usedPorts = new HashSet<int>();
		lock (m_instanceLock)
		{
			foreach (var inst in m_instances.Values)
			{
				if (!inst.IsServer && inst.ClientGamePort > 0)
					usedPorts.Add(inst.ClientGamePort);
			}
		}
		for (int port = 25100; port < 25200; port += 2)
		{
			if (usedPorts.Contains(port)) continue;
			// double-check the udp port is actually free at the OS level
			try
			{
				using var udp = new System.Net.Sockets.UdpClient(port);
				udp.Close();
				return port;
			}
			catch { }
		}
		return 25100;
	}

	private int FindFreeServerGamePort()
	{
		var usedPorts = new HashSet<int>();
		lock (m_instanceLock)
		{
			foreach (var inst in m_instances.Values)
			{
				if (inst.IsServer && inst.ServerGamePort > 0)
					usedPorts.Add(inst.ServerGamePort);
			}
		}
		for (int port = 25200; port < 25300; port += 2)
		{
			if (usedPorts.Contains(port)) continue;
			try
			{
				using var udp = new System.Net.Sockets.UdpClient(port);
				udp.Close();
				return port;
			}
			catch { }
		}
		return 25200;
	}

	private void LaunchGame(string exeName, string args, bool isServer = false, string level = "", JObject? msg = null)
	{
		string workingDir = isServer
			? GetServerDataDir(m_selectedGame)
			: m_gameDirectory;
		if (isServer) Directory.CreateDirectory(workingDir);

		var startInfo = new ProcessStartInfo
		{
			FileName = Path.Combine(m_gameDirectory, exeName),
			WorkingDirectory = workingDir,
			Arguments = args,
			UseShellExecute = false,
			RedirectStandardOutput = true,
			RedirectStandardInput = true,
			RedirectStandardError = true,
			StandardOutputEncoding = Encoding.UTF8,
			CreateNoWindow = true
		};
		startInfo.Environment["CYPRESS_EMBEDDED"] = "1";
		bool emancipated = (bool)(msg?["emancipated"] ?? false);
		if (!emancipated)
			startInfo.Environment["CYPRESS_MASTER_URL"] = MASTER_SERVER_URL;

		int sideChannelPort = 14638;
		if (isServer)
		{
			sideChannelPort = FindFreeSideChannelPort();
			startInfo.Environment["CYPRESS_SIDE_CHANNEL_PORT"] = sideChannelPort.ToString();
		}

		// assign a unique game port per client so multiple clients don't collide on 25100
		int clientGamePort = 0;
		if (!isServer)
		{
			clientGamePort = FindFreeClientGamePort();
			startInfo.Environment["CYPRESS_CLIENT_PORT"] = clientGamePort.ToString();
		}

		// assign a unique game port per server so multiple servers don't collide on 25200
		int serverGamePort = 0;
		if (isServer)
		{
			serverGamePort = FindFreeServerGamePort();
			startInfo.Environment["CYPRESS_SERVER_PORT"] = serverGamePort.ToString();

			// optional: block ID_ prefixed usernames
			if ((bool)(msg?["blockIdNames"] ?? false))
				startInfo.Environment["CYPRESS_BLOCK_ID_NAMES"] = "1";

			if (!emancipated)
			{
				// require players to have a linked Cypress account (default: on)
				if ((bool)(msg?["requireCypressAccount"] ?? true))
					startInfo.Environment["CYPRESS_REQUIRE_IDENTITY"] = "1";

				// allow global moderators to moderate this server (default: on)
				if (!(bool)(msg?["allowGlobalMods"] ?? true))
					startInfo.Environment["CYPRESS_ALLOW_GLOBAL_MODS"] = "0";
			}

			// unified banlist path (shared across all games)
			startInfo.Environment["CYPRESS_BANLIST_PATH"] = GetUnifiedBanlistPath();
		}

#if WINDOWS
		SetGpuPreferenceHighPerformance(startInfo.FileName);
#endif
		var process = new Process { StartInfo = startInfo };
		string game = m_selectedGame.ToString();
		PVZGame capturedGame = m_selectedGame;
		string capturedGameDir = m_gameDirectory;

		try
		{
			process.Start();
		}
		catch (System.ComponentModel.Win32Exception ex) when (ex.NativeErrorCode == 2)
		{
			ClearProxyEnvironment();
			SendStatus("Game executable not found.", "error");
			return;
		}
		catch (Exception ex)
		{
			ClearProxyEnvironment();
			SendStatus("Failed to launch: " + ex.Message, "error");
			return;
		}

		var instance = new GameInstance(
			process, game, isServer, clientGamePort, serverGamePort,
			onOutput: (pid, line) =>
			{
				try
				{
					Send(new JObject { ["type"] = "instanceOutput", ["pid"] = pid, ["line"] = line });
					// track player count for heartbeat + global ban check
					if (isServer && line.StartsWith('{'))
					{
						try
						{
							var j = JObject.Parse(line);
							var t = (string?)j["t"];
							var pname = (string?)j["name"];
							if (t == "playerJoin") UpdateHeartbeatPlayer(pid, pname, joined: true);
							else if (t == "playerLeave") UpdateHeartbeatPlayer(pid, pname, joined: false);
							else if (t == "sideChannelAuth")
							{
								string? name = (string?)j["name"];
								string? displayName = (string?)j["display_name"];
								string? extra = (string?)j["extra"];
								string? accountId = (string?)j["account_id"];
								// forward display_name + account_id to frontend (no ea_pid or hwid for hosters)
								j["display_name"] = displayName ?? name ?? "";
								j["account_id"] = accountId ?? "";
							}
						}
						catch { }
					}
				}
				catch { }
			},
			onExit: (pid) =>
			{
				int exitCode = 0;
				try { exitCode = process.ExitCode; } catch { }

				lock (m_instanceLock)
				{
					if (m_instances.Remove(pid, out var inst))
						inst.Dispose();
				}

				if (isServer) StopHeartbeat(pid);

				bool isLastForThisGame;
				lock (m_instanceLock)
				{
					isLastForThisGame = !m_instances.Values.Any(i => i.Game == game);
				}

				Environment.SetEnvironmentVariable("EARtPLaunchCode", null);

				string contentId = "0";
				if (capturedGame == PVZGame.GW1)
					contentId = "1011216";
				else if (capturedGame == PVZGame.GW2)
					contentId = "1026482";
				else if (capturedGame == PVZGame.BFN)
					contentId = "1036445";

				Environment.SetEnvironmentVariable("ContentId", contentId);

				Environment.SetEnvironmentVariable("GW_LAUNCH_ARGS", null);
				Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_JWT", null);
				Environment.SetEnvironmentVariable("CYPRESS_IDENTITY_KEY", null);
				ClearProxyEnvironment();
				if (capturedGame < PVZGame.BFN)
				{
					try { File.Delete(Path.Combine(capturedGameDir, "CryptBase.dll")); } catch { }
				}

				if (isLastForThisGame)
				{
					try { File.Delete(Path.Combine(capturedGameDir, s_destDLLName)); } catch { }
				}

				try
				{
					Send(new JObject { ["type"] = "instanceExited", ["pid"] = pid, ["exitCode"] = exitCode });
					SendStatus($"Game exited with code {exitCode:X}", "info");
				}
				catch { }
			}
		);

		lock (m_instanceLock)
		{
			m_instances[instance.Pid] = instance;
		}

		if (isServer)
		{
			string logDir = Path.Combine(
				Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
				"Cypress", "Logs");
			string logName = $"{game}_{instance.Pid}_{DateTime.Now:yyyyMMdd_HHmmss}.log";
			instance.EnableFileLog(Path.Combine(logDir, logName));
		}

		var instanceMsg = new JObject
		{
			["type"] = "instanceStarted",
			["pid"] = instance.Pid,
			["game"] = game,
			["isServer"] = isServer,
			["level"] = level,
			["startTime"] = instance.StartTime.ToString("o")
		};

		if (isServer)
		{
			string motd = ((string?)msg?["serverName"]) ?? "";
			string icon = ((string?)msg?["serverIcon"]) ?? "";
			bool modded = (bool)(msg?["useMods"] ?? false);
			string modpackUrl = SanitizeModpackUrl((string?)msg?["modpackUrl"]);
			if (!string.IsNullOrEmpty(motd)) instanceMsg["motd"] = motd;
			if (!string.IsNullOrEmpty(icon)) instanceMsg["icon"] = icon;
			instanceMsg["modded"] = modded;
			if (!string.IsNullOrEmpty(modpackUrl)) instanceMsg["modpackUrl"] = modpackUrl;
		}
		else
		{
			string username = ((string?)msg?["username"]) ?? "";
			if (!string.IsNullOrEmpty(username)) instanceMsg["username"] = username;
		}

		Send(instanceMsg);
		SendStatus($"Game launched (PID {instance.Pid})", "success");

		// push mod token via stdin so client can claim mod on connect
		if (!isServer && !string.IsNullOrEmpty(m_modToken))
			instance.SendCommand("Cypress.SetModToken " + m_modToken);

		if (isServer)
		{
			string serverName = ((string?)msg?["serverName"]) ?? "";
			string serverIcon = ((string?)msg?["serverIcon"]) ?? "";
			bool useMods = (bool)(msg?["useMods"] ?? false);
			string modpackUrl = SanitizeModpackUrl((string?)msg?["modpackUrl"]);
			if (!string.IsNullOrEmpty(serverName) || !string.IsNullOrEmpty(serverIcon) || useMods || !string.IsNullOrEmpty(modpackUrl))
			{
				var infoJson = new JObject();
				if (!string.IsNullOrEmpty(serverName)) infoJson["motd"] = serverName;
				if (!string.IsNullOrEmpty(serverIcon)) infoJson["icon"] = serverIcon;
				infoJson["modded"] = useMods;
				if (!string.IsNullOrEmpty(modpackUrl)) infoJson["modpackUrl"] = modpackUrl;
				instance.SendCommand("Cypress.SetServerInfo " + infoJson.ToString(Newtonsoft.Json.Formatting.None));
			}

			string heartbeatAddr = ((string?)msg?["deviceIP"]) ?? "";
			if (string.IsNullOrWhiteSpace(heartbeatAddr)) heartbeatAddr = TryGetPreferredDeviceIp();
			int launchedPid = instance.Pid;
			_ = Task.Run(async () =>
			{
				int actualPort = await ReadDiscoveryPortAsync(launchedPid);

				var heartbeatData = new JObject
				{
					["address"] = heartbeatAddr,
					["port"] = actualPort,
					["game"] = game,
					["maxPlayers"] = int.TryParse(((string?)msg?["playerCount"]) ?? "", out var mp) ? mp : 24,
					["level"] = level,
					["gamePort"] = serverGamePort,
				};
				string inclusion = ((string?)msg?["inclusion"]) ?? "";
				if (!string.IsNullOrEmpty(inclusion))
				{
					// extract GameMode from inclusion string (e.g. "GameMode=FreeRoam;HostedMode=ServerHosted")
					var modeId = inclusion.Split(';')
						.Select(p => p.Split('='))
						.Where(kv => kv.Length == 2 && kv[0] == "GameMode")
						.Select(kv => kv[1])
						.FirstOrDefault();
					if (!string.IsNullOrEmpty(modeId))
						heartbeatData["mode"] = modeId;
					else
						heartbeatData["mode"] = inclusion;
				}
				if (!string.IsNullOrEmpty(serverName)) heartbeatData["motd"] = serverName;
				if (!string.IsNullOrEmpty(serverIcon)) heartbeatData["icon"] = serverIcon;
				if (useMods) heartbeatData["modded"] = true;
				if (!string.IsNullOrEmpty(modpackUrl)) heartbeatData["modpackUrl"] = modpackUrl;
				if (!string.IsNullOrWhiteSpace(((string?)msg?["dedicatedPassword"]) ?? "")) heartbeatData["hasPassword"] = true;

				string vpnType = ((string?)msg?["vpnType"]) ?? "";
				if (!string.IsNullOrEmpty(vpnType))
				{
					heartbeatData["vpnType"] = vpnType;
					heartbeatData["vpnNetwork"] = ((string?)msg?["vpnNetwork"]) ?? "";
					heartbeatData["vpnPassword"] = ((string?)msg?["vpnPassword"]) ?? "";
				}

				// include relay info so browser players can auto-join
				string hbHostMode = ((string?)msg?["hostConnectionMode"]) ?? "Direct";
				if (string.Equals(hbHostMode, "Relay", StringComparison.OrdinalIgnoreCase))
				{
					string hbRelayAddr = ((string?)msg?["hostRelayAddress"]) ?? "";
					string hbRelayKey = ((string?)msg?["hostRelayKey"]) ?? "";
					string hbRelayCode = ((string?)msg?["hostRelayCode"]) ?? "";
					if (!string.IsNullOrEmpty(hbRelayAddr)) heartbeatData["relayAddress"] = hbRelayAddr;
					if (!string.IsNullOrEmpty(hbRelayKey)) heartbeatData["relayKey"] = hbRelayKey;
					if (!string.IsNullOrEmpty(hbRelayCode)) heartbeatData["relayCode"] = hbRelayCode;
				}

		bool listedInBrowser = !emancipated && (bool)(msg?["listedInBrowser"] ?? true);
				if (listedInBrowser)
					StartHeartbeat(heartbeatData, launchedPid);
			});
		}
	}

#if WINDOWS
	[System.Runtime.Versioning.SupportedOSPlatform("windows")]
	private static void SetGpuPreferenceHighPerformance(string exePath)
	{
		try
		{
			using var key = Microsoft.Win32.Registry.CurrentUser.CreateSubKey(
				@"SOFTWARE\Microsoft\DirectX\UserGpuPreferences", writable: true);
			key?.SetValue(exePath, "GpuPreference=2;", Microsoft.Win32.RegistryValueKind.String);
		}
		catch { }
	}
#endif
}
