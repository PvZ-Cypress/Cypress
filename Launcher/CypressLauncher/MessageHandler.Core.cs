#nullable enable
using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Net.NetworkInformation;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using Newtonsoft.Json.Linq;
using Photino.NET;

namespace CypressLauncher;

public partial class MessageHandler
{
	public enum PVZGame
	{
		GW1,
		GW2,
		BFN
	}

	private static readonly string s_destDLLName = "dinput8.dll";
	private static readonly string s_patchedGW2ExeName = "GW2.PreEAAC.exe";
	private static readonly string s_patchedBFNExeName = "BFN.PreEAAC.exe";
	private static readonly string s_launcherSavedataFilename = "launcherdata.json";
	private static readonly HttpClient s_httpClient = new() { Timeout = TimeSpan.FromSeconds(15) };
	private const int s_defaultProxyPort = 25201;
	private const int s_defaultProxyApiPort = 8080;
	private const string s_proxyAddressEnv = "CYPRESS_PROXY_ADDRESS";
	private const string s_proxyPortEnv = "CYPRESS_PROXY_PORT";
	private const string s_proxyKeyEnv = "CYPRESS_PROXY_KEY";

	internal static readonly Dictionary<PVZGame, string> s_gameToExecutableName = new()
	{
		{ PVZGame.GW1, "PVZ.Main_Win64_Retail.exe" },
		{ PVZGame.GW2, "GW2.Main_Win64_Retail.exe" },
		{ PVZGame.BFN, "PVZBattleforNeighborville.exe" }
	};

	internal static readonly Dictionary<PVZGame, string> s_gameToPatchedExecutableName = new()
	{
		{ PVZGame.GW2, s_patchedGW2ExeName },
		{ PVZGame.BFN, s_patchedBFNExeName }
	};

	private static readonly Dictionary<PVZGame, string> s_gameToGameName = new()
	{
		{ PVZGame.GW1, "Garden Warfare 1" },
		{ PVZGame.GW2, "Garden Warfare 2" },
		{ PVZGame.BFN, "Battle for Neighborville" }
	};

	private static readonly Dictionary<PVZGame, string> s_specialLaunchArgsForGame = new()
	{
		{ PVZGame.GW1, "-GameTime.MaxSimFps -1" },
		{ PVZGame.GW2, "-GameMode.SkipIntroHubNIS true" },
		{ PVZGame.BFN, "-GameMode.ShouldSkipHUBTutorial 1 -GameMode.SocialHUBSkipStationTutorials 1 " }
	};

	private static readonly Dictionary<PVZGame, string> s_serverLaunchArgsForGame = new()
	{
		{ PVZGame.GW1, "-Online.ClientIsPresenceEnabled false -Online.ServerIsPresenceEnabled false -Online.Backend Backend_Peer -Online.PeerBackend Backend_Peer -Server.IsRanked false -Game.Platform GamePlatform_Win32 -SyncedBFSettings.AllUnlocksUnlocked true -PingSite ams -name \"PVZGW Dedicated Server\"  " },
		{ PVZGame.GW2, "-enableServerLog -platform Win32 -console -Game.Platform GamePlatform_Win32 -Game.EnableServerLog true -GameMode.SkipIntroHubNIS true -Online.Backend Backend_Local -Online.PeerBackend Backend_Local -PVZServer.MapSequencerEnabled false " },
		{ PVZGame.BFN, "-Online.ClientIsPresenceEnabled 0 -Online.ServerIsPresenceEnabled 0 -Game.Platform GamePlatform_Win32 -allUnlocksUnlocked -GameMode.OverrideRoundStartPlayerCount 1 -Online.Backend Backend_Local -Online.PeerBackend Backend_Local -PVZServer.MapSequencerEnabled false " }
	};

	private PVZGame m_selectedGame = PVZGame.GW2;
	private string m_gameDirectory = string.Empty;

	private static string GetServerDataDir(PVZGame game) =>
		Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Cypress", "ServerData", game.ToString());

	private static string GetUnifiedBanlistPath()
	{
		var dir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Cypress", "ServerData");
		Directory.CreateDirectory(dir);
		return Path.Combine(dir, "bans.json");
	}

	private PhotinoWindow? m_window;
	private readonly Dictionary<int, GameInstance> m_instances = new();
	private readonly Dictionary<int, ExternalInstance> m_externalInstances = new();
	private readonly object m_instanceLock = new();
	private long m_lastLaunchTicks = 0;

	private static string MASTER_SERVER_URL => LauncherConfig.MasterUrl;
	private readonly Dictionary<int, HeartbeatState> m_heartbeats = new();

	private sealed class HeartbeatState
	{
		public JObject Data;
		public string? Token;
		public int Count;
		public int PlayerCount;
		public HashSet<string> PlayerNames = new();
		public System.Threading.Timer? Timer;
		public HeartbeatState(JObject data) { Data = data; }
	}

	public PhotinoWindow? Window => m_window;
	public object? TrayIcon { get; set; }

	private void ShowTrayIcon()
	{
		if (TrayIcon != null && RuntimeInformation.IsOSPlatform(OSPlatform.Windows))
			((dynamic)TrayIcon).Visible = true;
	}

	private sealed record ProxyEndpoint(string Host, int Port);

	private string GetAppdataDir()
	{
		string dir = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Cypress");
		Directory.CreateDirectory(dir);
		return dir;
	}

	private string GetServerDLLName() => $"cypress_{m_selectedGame}.dll";
	private string GetServerDLLPath() => Path.Combine(AppContext.BaseDirectory, GetServerDLLName());

	// block until all deregister requests finish (up to 5s)
	private void ShutdownHeartbeats()
	{
		try { StopAllHeartbeatsAsync().Wait(TimeSpan.FromSeconds(5)); } catch { }
	}

	public void DeregisterAllHeartbeats()
	{
		try { StopAllHeartbeatsAsync().Wait(TimeSpan.FromSeconds(3)); } catch { }
	}

	public void KillAllInstances(bool skipHeartbeatShutdown = false)
	{
		if (!skipHeartbeatShutdown) ShutdownHeartbeats();
		lock (m_instanceLock)
		{
			foreach (var kvp in m_instances)
			{
				try { kvp.Value.Kill(); } catch { }
				try { kvp.Value.Dispose(); } catch { }
			}
			m_instances.Clear();
		}
	}

	private bool GameRequiresPatchedExe(string filepath, ref bool failed)
	{
		try
		{
			DateTimeOffset timestamp = DateTimeOffset.FromUnixTimeSeconds(PEFile.GetNTHeaderFromPE(filepath).TimeDateStamp);
			failed = false;
			return m_selectedGame switch
			{
				PVZGame.GW1 => false,
				PVZGame.GW2 or PVZGame.BFN => timestamp.Year >= 2024,
				_ => false
			};
		}
		catch (Exception ex)
		{
			SendStatus("Exception checking executable: " + ex.Message, "error");
			failed = true;
			return false;
		}
	}

	private uint GetRtPLaunchCode()
	{
		DateTime utcNow = DateTime.UtcNow;
		uint num = (uint)((utcNow.Year * 104729) ^ (utcNow.Month * 224737) ^ (utcNow.Day * 350377));
		return num ^ ((num << 16) ^ (num >> 16));
	}

	private void Send(JObject data)
	{
		m_window?.SendWebMessage(data.ToString(Newtonsoft.Json.Formatting.None));
	}

	private void SendStatus(string text, string level = "info")
	{
		Send(new JObject { ["type"] = "status", ["text"] = text, ["level"] = level });
	}

	public void HandleMessage(object? sender, string message)
	{
		m_window = sender as PhotinoWindow;
		try
		{
			JObject msg = JObject.Parse(message);
			string? type = (string?)msg["type"];
			switch (type)
			{
				case "checkTos":
					OnCheckTos();
					break;
				case "acceptTos":
					OnAcceptTos();
					break;
				case "checkAuth":
					OnCheckAuth();
					break;
				case "eaLogin":
					OnEaLogin();
					break;
				case "eaLogout":
					OnEaLogout();
					break;
				case "getAuthTicket":
					OnGetAuthTicket();
					break;
				case "init":
					OnInit();
					break;
				case "gameChanged":
					OnGameChanged((string?)msg["game"] ?? "GW2");
					break;
				case "selectGameDir":
					OnSelectGameDir();
					break;
				case "autoFindDir":
					OnAutoFindDir();
					break;
				case "getModPacks":
					OnGetModPacks();
					break;
				case "getPlaylists":
					OnGetPlaylists();
					break;
				case "savePlaylist":
					OnSavePlaylist((string?)msg["name"] ?? "", (string?)msg["content"] ?? "");
					break;
				case "loadPlaylist":
					OnLoadPlaylist((string?)msg["name"] ?? "");
					break;
				case "deletePlaylist":
					OnDeletePlaylist((string?)msg["name"] ?? "");
					break;
				case "getRelayLease":
					OnGetRelayLease(msg);
					break;
				case "resolveRelayCode":
					OnResolveRelayCode(msg);
					break;
				case "join":
					OnJoin(msg);
					break;
				case "startServer":
					OnStartServer(msg);
					break;
				case "openExternal":
					OnOpenExternal((string?)msg["url"]);
					break;
				case "windowMinimize":
					m_window?.SetMinimized(true);
					break;
				case "windowMaximize":
					m_window?.SetMaximized(m_window?.Maximized != true);
					break;
				case "windowClose":
					m_window?.SetMinimized(true);
					ShowTrayIcon();
					break;
				case "windowDragStart":
					if (m_window != null)
						Send(new JObject { ["type"] = "windowDragStart", ["windowX"] = m_window.Left, ["windowY"] = m_window.Top });
					break;
				case "windowDragMove":
					if (m_window != null)
					{
						m_window.SetLeft((int)(msg["x"] ?? 0));
						m_window.SetTop((int)(msg["y"] ?? 0));
					}
					break;
				case "windowToTray":
					m_window?.SetMinimized(true);
					ShowTrayIcon();
					break;
				case "getMapBg":
					OnGetMapBg((string?)msg["key"] ?? "");
					break;
				case "getModeBg":
					OnGetModeBg((string?)msg["key"] ?? "");
					break;
				case "getCharIcon":
					OnGetCharIcon((string?)msg["key"] ?? "");
					break;
				case "getAiSetBg":
					OnGetAiSetBg((string?)msg["key"] ?? "");
					break;
				case "sendCommand":
					OnSendCommand((int)(msg["pid"] ?? 0), (string?)msg["cmd"] ?? "");
					break;
				case "killInstance":
					OnKillInstance((int)(msg["pid"] ?? 0));
					break;
				case "getInstances":
					OnGetInstances();
					break;
				case "checkServer":
					OnCheckServer((string?)msg["address"] ?? "", (string?)msg["relayAddress"], (string?)msg["relayKey"]);
					break;
				case "saveServerList":
					OnSaveServerList(msg["servers"] as JArray);
					break;
				case "saveProfile":
					SaveProfileData(msg);
					break;
				case "fetchBrowser":
					OnFetchBrowser();
					break;
				case "fetchBrowserIcon":
					OnFetchBrowserIcon((string?)msg["key"] ?? "");
					break;
				case "attachInstance":
					OnAttachInstance((string?)msg["address"] ?? "127.0.0.1", (int)(msg["port"] ?? 14638), (int)(msg["pid"] ?? 0));
					break;
				case "detachInstance":
					OnDetachInstance((int)(msg["pid"] ?? 0));
					break;
				case "detectInstances":
					OnDetectInstances();
					break;
				case "modLogin":
					OnModLogin(msg);
					break;
				case "modLogout":
					OnModLogout();
					break;
				case "modGlobalBan":
					OnModGlobalBan(msg);
					break;
				case "modGlobalUnban":
					OnModGlobalUnban(msg);
					break;
				case "modGetGlobalBans":
					OnModGetGlobalBans();
					break;
				case "modBanServer":
					OnModBanServer(msg);
					break;
				case "modBanServerByKey":
					OnModBanServerByKey(msg);
					break;
				case "modUnbanServer":
					OnModUnbanServer(msg);
					break;
				case "modGetBannedServers":
					OnModGetBannedServers();
					break;
				case "setUseGlobalBanDB":
					m_useGlobalBanDB = (bool)(msg["enabled"] ?? true);
					break;
				case "modGlobalBanPlayer":
					OnModGlobalBanPlayer(msg);
					break;
				case "modBanByPid":
					OnModBanByPid(msg);
					break;
				case "modPinServer":
					OnModPinServer(msg);
					break;
				case "modUnpinServer":
					OnModUnpinServer(msg);
					break;
				case "getLocalBans":
					OnGetLocalBans();
					break;
				case "checkIdentity":
					OnCheckIdentity();
					break;
				case "clearIdentity":
					OnClearIdentity();
					break;
				case "registerIdentity":
					OnRegisterIdentity(msg);
					break;
				case "rebindIdentity":
					OnRebindIdentity(msg);
					break;
				case "setNickname":
					OnSetNickname(msg);
					break;
				case "refreshEntitlements":
					OnRefreshEntitlements();
					break;
				case "relinkEA":
					OnRelinkEA();
					break;
				case "checkUpdates":
					OnCheckUpdates();
					break;
				case "startUpdate":
					OnStartUpdate(msg);
					break;
				case "getTranslations":
					OnGetTranslations((string?)msg["lang"] ?? "en_us");
					break;
				case "getTranslationsList":
					OnGetTranslationsList();
					break;
			}
		}
		catch (Exception ex)
		{
			SendStatus("Error: " + ex.Message, "error");
		}
	}
}
