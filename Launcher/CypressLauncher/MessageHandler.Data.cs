#nullable enable
using System;
using System.IO;
using Newtonsoft.Json.Linq;

namespace CypressLauncher;

public partial class MessageHandler
{
	private void SaveProfileData(JObject msg)
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			JObject root = new JObject();
			if (File.Exists(filePath))
				root = JObject.Parse(File.ReadAllText(filePath));

			if (msg["username"] != null) root["Username"] = (string?)msg["username"];
			if (msg["fov"] != null) root["FOV"] = (string?)msg["fov"];
			if (msg["additionalArgs"] != null) root["AdditionalLaunchArgs"] = (string?)msg["additionalArgs"];
			if (msg["darkMode"] != null) root["DarkMode"] = (bool?)msg["darkMode"] ?? false;

			File.WriteAllText(filePath, root.ToString());
		}
		catch { }
	}

	private void SaveCurrentFormData(JObject msg)
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			JObject root = new JObject();
			if (File.Exists(filePath))
				root = JObject.Parse(File.ReadAllText(filePath));

			string game = m_selectedGame.ToString();
			root["SelectedGame"] = game;

			// profile fields are global now
			if (msg["username"] != null) root["Username"] = (string?)msg["username"];
			if (msg["fov"] != null) root["FOV"] = (string?)msg["fov"];
			if (msg["additionalArgs"] != null) root["AdditionalLaunchArgs"] = (string?)msg["additionalArgs"];

			JObject profile = root[game] as JObject ?? new JObject();
			if (msg["serverIP"] != null) profile["ServerIP"] = (string?)msg["serverIP"];
			if (msg["joinConnectionMode"] != null) profile["JoinConnectionMode"] = (string?)msg["joinConnectionMode"];
			if (msg["joinRelayPreset"] != null) profile["JoinRelayPreset"] = (string?)msg["joinRelayPreset"];
			if (msg["joinRelayAddress"] != null) profile["JoinRelayAddress"] = (string?)msg["joinRelayAddress"];
			if (msg["joinRelayKey"] != null) profile["JoinRelayKey"] = (string?)msg["joinRelayKey"];
			profile["GameDirectory"] = m_gameDirectory;
			if (msg["serverPassword"] != null) profile["ServerPassword"] = (string?)msg["serverPassword"];
			if (msg["deviceIP"] != null) profile["DeviceIP"] = (string?)msg["deviceIP"];
			if (msg["hostConnectionMode"] != null) profile["HostConnectionMode"] = (string?)msg["hostConnectionMode"];
			if (msg["hostRelayPreset"] != null) profile["HostRelayPreset"] = (string?)msg["hostRelayPreset"];
			if (msg["hostRelayAddress"] != null) profile["HostRelayAddress"] = (string?)msg["hostRelayAddress"];
			if (msg["hostRelayKey"] != null) profile["HostRelayKey"] = (string?)msg["hostRelayKey"];
			if (msg["hostRelayServerName"] != null) profile["HostRelayServerName"] = (string?)msg["hostRelayServerName"];
			if (msg["hostRelayJoinLink"] != null) profile["HostRelayJoinLink"] = (string?)msg["hostRelayJoinLink"];
			if (msg["hostRelayCode"] != null) profile["HostRelayCode"] = (string?)msg["hostRelayCode"];
			if (msg["level"] != null) profile["Level"] = (string?)msg["level"];
			if (msg["inclusion"] != null) profile["Inclusion"] = (string?)msg["inclusion"];
			if (msg["dedicatedPassword"] != null) profile["DedicatedServerPassword"] = (string?)msg["dedicatedPassword"];
			if (msg["playerCount"] != null) profile["PlayerCount"] = (string?)msg["playerCount"];
			if (msg["startPoint"] != null) profile["StartPoint"] = (string?)msg["startPoint"];
			if (msg["serverName"] != null) profile["ServerName"] = (string?)msg["serverName"];
			if (msg["serverIcon"] != null) profile["ServerIcon"] = (string?)msg["serverIcon"];
			root[game] = profile;

			File.WriteAllText(filePath, root.ToString());
		}
		catch { }
	}

	private void SaveUserData(string? previousGame)
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			JObject root = new JObject();
			if (File.Exists(filePath))
				root = JObject.Parse(File.ReadAllText(filePath));

			if (previousGame != null)
			{
				JObject profile = root[previousGame] as JObject ?? new JObject();
				profile["GameDirectory"] = m_gameDirectory;
				root[previousGame] = profile;
			}
			root["SelectedGame"] = m_selectedGame.ToString();
			File.WriteAllText(filePath, root.ToString());
		}
		catch { }
	}

	private void GetLastSelectedGame(out PVZGame selectedGame)
	{
		string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
		if (File.Exists(filePath))
		{
			try
			{
				JObject root = JObject.Parse(File.ReadAllText(filePath));
				if (root.ContainsKey("SelectedGame") && Enum.TryParse<PVZGame>((string?)root["SelectedGame"], out PVZGame result))
				{
					selectedGame = result;
					return;
				}
			}
			catch { }
		}
		selectedGame = PVZGame.GW2;
	}

	private void LoadAndSendUserData(string? profileName)
	{
		string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
		JObject response = new JObject { ["type"] = "loadUserData" };
		response["game"] = m_selectedGame.ToString();
		string detectedDeviceIp = TryGetPreferredDeviceIp();
		if (!string.IsNullOrWhiteSpace(detectedDeviceIp))
			response["detectedDeviceIP"] = detectedDeviceIp;
		response["relayServers"] = new JObject
		{
			["na"] = LauncherConfig.RelayNA,
			["eu"] = LauncherConfig.RelayEU
		};

		if (File.Exists(filePath))
		{
			try
			{
				JObject root = JObject.Parse(File.ReadAllText(filePath));

				// profile fields are global (root level)
				if (root["Username"] != null) response["username"] = (string?)root["Username"];
				if (root["FOV"] != null) response["fov"] = (string?)root["FOV"];
				if (root["AdditionalLaunchArgs"] != null) response["additionalArgs"] = (string?)root["AdditionalLaunchArgs"];
				if (root["DarkMode"] != null) response["darkMode"] = (bool?)root["DarkMode"] ?? false;
				if (root["DarkMode"] != null) response["darkMode"] = (bool?)root["DarkMode"] ?? false;

				string name = profileName ?? m_selectedGame.ToString();
				if (root[name] is JObject profile)
				{
					// migrate old per-game profile fields to root
					bool migrated = false;
					if (root["Username"] == null && profile["Username"] != null) { root["Username"] = profile["Username"]; response["username"] = (string?)profile["Username"]; migrated = true; }
					if (root["FOV"] == null && profile["FOV"] != null) { root["FOV"] = profile["FOV"]; response["fov"] = (string?)profile["FOV"]; migrated = true; }
					if (root["AdditionalLaunchArgs"] == null && profile["AdditionalLaunchArgs"] != null) { root["AdditionalLaunchArgs"] = profile["AdditionalLaunchArgs"]; response["additionalArgs"] = (string?)profile["AdditionalLaunchArgs"]; migrated = true; }
					if (migrated) try { File.WriteAllText(filePath, root.ToString()); } catch { }

					if (profile["ServerIP"] != null) response["serverIP"] = (string?)profile["ServerIP"];
					if (profile["JoinConnectionMode"] != null) response["joinConnectionMode"] = (string?)profile["JoinConnectionMode"];
					if (profile["JoinRelayPreset"] != null) response["joinRelayPreset"] = (string?)profile["JoinRelayPreset"];
					if (profile["JoinRelayAddress"] != null) response["joinRelayAddress"] = (string?)profile["JoinRelayAddress"];
					if (profile["JoinRelayKey"] != null) response["joinRelayKey"] = (string?)profile["JoinRelayKey"];
					response["gameDir"] = (string?)profile["GameDirectory"] ?? "";
					m_gameDirectory = (string?)profile["GameDirectory"] ?? "";
					if (profile["ServerPassword"] != null) response["serverPassword"] = (string?)profile["ServerPassword"];
					if (profile["DeviceIP"] != null) response["deviceIP"] = (string?)profile["DeviceIP"];
					if (profile["HostConnectionMode"] != null) response["hostConnectionMode"] = (string?)profile["HostConnectionMode"];
					if (profile["HostRelayPreset"] != null) response["hostRelayPreset"] = (string?)profile["HostRelayPreset"];
					if (profile["HostRelayAddress"] != null) response["hostRelayAddress"] = (string?)profile["HostRelayAddress"];
					if (profile["HostRelayKey"] != null) response["hostRelayKey"] = (string?)profile["HostRelayKey"];
					if (profile["HostRelayServerName"] != null) response["hostRelayServerName"] = (string?)profile["HostRelayServerName"];
					if (profile["HostRelayJoinLink"] != null) response["hostRelayJoinLink"] = (string?)profile["HostRelayJoinLink"];
					if (profile["HostRelayCode"] != null) response["hostRelayCode"] = (string?)profile["HostRelayCode"];
					if (profile["Level"] != null) response["level"] = (string?)profile["Level"];
					if (profile["Inclusion"] != null) response["inclusion"] = (string?)profile["Inclusion"];
					if (profile["DedicatedServerPassword"] != null) response["dedicatedPassword"] = (string?)profile["DedicatedServerPassword"];
					if (profile["PlayerCount"] != null) response["playerCount"] = (string?)profile["PlayerCount"];
					if (profile["StartPoint"] != null) response["startPoint"] = (string?)profile["StartPoint"];
					if (profile["ServerName"] != null) response["serverName"] = (string?)profile["ServerName"];
					if (profile["ServerIcon"] != null) response["serverIcon"] = (string?)profile["ServerIcon"];
				}

				if (root["ServerList"] is JArray sl)
					response["serverList"] = sl;
			}
			catch
			{
				try { File.Delete(filePath); } catch { }
			}
		}

		if (response["deviceIP"] == null && !string.IsNullOrWhiteSpace(detectedDeviceIp))
			response["deviceIP"] = detectedDeviceIp;

		if (response["gameDir"] == null)
		{
			response["gameDir"] = "";
			m_gameDirectory = "";
		}

		if (string.IsNullOrEmpty(m_gameDirectory))
		{
			string? found = TrySilentAutoFindDir();
			if (found != null)
			{
				m_gameDirectory = found;
				response["gameDir"] = found;
			}
		}

		Send(response);
	}
}
