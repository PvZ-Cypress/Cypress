#nullable enable
using System;
using System.Collections.Generic;
using System.IO;
using System.Net.Http;
using System.Security.Cryptography;
using System.Text;
using System.Linq;
using System.Threading.Tasks;
using Newtonsoft.Json.Linq;
#if WINDOWS
using Microsoft.Win32;
#endif

namespace CypressLauncher;

public partial class MessageHandler
{
	private const string EA_CLIENT_ID = "JUNO_PC_CLIENT";
	private const string EA_CLIENT_SECRET = "4mRLtYMb6vq9qglomWEaT4ChxsXWcyqbQpuBNfMPOYOiDmYYQmjuaBsF2Zp0RyVeWkfqhE9TuGgAw7te";
	private const string EA_AUTH_URL = "https://accounts.ea.com/connect/auth";
	private const string EA_TOKEN_URL = "https://accounts.ea.com/connect/token";
	private const string EA_REDIRECT_URI = "qrc:///html/login_successful.html";

	// persisted auth state
	private string? m_authToken;
	private string? m_authPid;
	private string? m_authUid;
	private string? m_authDisplayName;
	private double m_authExpiresAt;

	// gotta be kept in memory for entitlements/relink
	private string? m_eaAccessToken;

	// pkce state for in-flight login
	private string? m_codeVerifier;

	private void OnCheckAuth()
	{
		// A prepared Private Online Dynasty run uses a local Cypress persona and must
		// not validate or refresh an EA/master identity just to open the launcher.
		// This startup-only flag is set by the repository preparation script; the
		// game process has its own stricter PRIVATE_ONLINE_DYNASTY activation gate.
		if (string.Equals(
			Environment.GetEnvironmentVariable("CYPRESS_CFB27_PRIVATE_PREPARE"),
			"1",
			StringComparison.Ordinal))
		{
			Send(new JObject
			{
				["type"] = "authStatus",
				["loggedIn"] = false,
				["privateOnlineDynasty"] = true
			});
			return;
		}

		LoadAuthFromDisk();

		if (m_authToken != null && m_authExpiresAt > DateTimeOffset.UtcNow.ToUnixTimeSeconds())
		{
			// validate session is still good with master
			Task.Run(async () =>
			{
				try
				{
					var req = new HttpRequestMessage(HttpMethod.Get, MASTER_SERVER_URL + "/auth/me");
					req.Headers.Add("Authorization", "Bearer " + m_authToken);
					var resp = await s_httpClient.SendAsync(req);
					if (resp.IsSuccessStatusCode)
					{
						var body = JObject.Parse(await resp.Content.ReadAsStringAsync());
						m_authDisplayName = (string?)body["displayName"] ?? m_authDisplayName;
						m_authUid = (string?)body["uid"]?.ToString() ?? m_authUid;
						Send(new JObject { ["type"] = "authStatus", ["loggedIn"] = true, ["displayName"] = m_authDisplayName, ["pid"] = m_authPid, ["uid"] = m_authUid });
						await AutoRegisterIdentityAsync();
						OnModLogin(new JObject());
						return;
					}
				}
				catch { }

				// session expired or invalid
				ClearAuth();
				Send(new JObject { ["type"] = "authStatus", ["loggedIn"] = false });
			});
		}
		else
		{
			ClearAuth();
			Send(new JObject { ["type"] = "authStatus", ["loggedIn"] = false });
		}
	}

	private void OnEaLogin()
	{
		CleanupLegacyQrcHandler();

		var verifierBytes = new byte[32];
		RandomNumberGenerator.Fill(verifierBytes);
		m_codeVerifier = Base64UrlEncode(verifierBytes);

		byte[] challengeHash = SHA256.HashData(Encoding.ASCII.GetBytes(m_codeVerifier));
		string codeChallenge = Base64UrlEncode(challengeHash);

		string pcSign = GeneratePcSign();
		string authUrl = $"{EA_AUTH_URL}?client_id={EA_CLIENT_ID}"
			+ $"&response_type=code"
			+ $"&redirect_uri={Uri.EscapeDataString(EA_REDIRECT_URI)}"
			+ $"&code_challenge={codeChallenge}"
			+ $"&code_challenge_method=S256"
			+ $"&pc_sign={Uri.EscapeDataString(pcSign)}";

		if (TryStartEmbeddedEaLogin(authUrl))
			return;

		Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = "embedded ea login is unavailable on this platform" });
	}

	private void CleanupLegacyQrcHandler()
	{
#if WINDOWS
		try
		{
			Registry.CurrentUser.DeleteSubKeyTree(@"Software\Classes\qrc", throwOnMissingSubKey: false);
		}
		catch { }

		try
		{
			string handlerScript = Path.Combine(GetAppdataDir(), "qrc_handler.cmd");
			if (File.Exists(handlerScript))
				File.Delete(handlerScript);
		}
		catch { }
#endif
	}

	private bool TryStartEmbeddedEaLogin(string authUrl)
	{
#if WINDOWS
		try
		{
			Task.Run(async () =>
			{
				try
				{
					var result = await EaLoginWindow.ShowAsync(authUrl, EA_REDIRECT_URI);
					if (result.Cancelled)
					{
						Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = "Login cancelled" });
						return;
					}

					if (!string.IsNullOrEmpty(result.Error))
					{
						Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = result.Error });
						return;
					}

					if (string.IsNullOrEmpty(result.Code))
					{
						Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = "No authorization code received" });
						return;
					}

					await ExchangeCodeForToken(result.Code, EA_REDIRECT_URI);
				}
				catch (Exception ex)
				{
					Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = ex.Message });
				}
			});
			return true;
		}
		catch
		{
			return false;
		}
#else
		return false;
#endif
	}

	private async Task ExchangeCodeForToken(string code, string redirectUri)
	{
		try
		{
			// exchange auth code for EA JWT
			var tokenBody = new FormUrlEncodedContent(new[]
			{
				new KeyValuePair<string, string>("grant_type", "authorization_code"),
				new KeyValuePair<string, string>("code", code),
				new KeyValuePair<string, string>("client_id", EA_CLIENT_ID),
				new KeyValuePair<string, string>("client_secret", EA_CLIENT_SECRET),
				new KeyValuePair<string, string>("code_verifier", m_codeVerifier!),
				new KeyValuePair<string, string>("redirect_uri", redirectUri),
				new KeyValuePair<string, string>("token_format", "JWS"),
			});

			var tokenResp = await s_httpClient.PostAsync(EA_TOKEN_URL, tokenBody);
			string tokenRaw = await tokenResp.Content.ReadAsStringAsync();
			JObject? tokenJson = null;
			try { tokenJson = JObject.Parse(tokenRaw); } catch { }

			if (!tokenResp.IsSuccessStatusCode)
			{
				string err = (string?)tokenJson?["error_description"] ?? (tokenRaw.Length > 200 ? tokenRaw.Substring(0, 200) : tokenRaw);
				Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = $"EA token exchange failed: {err}" });
				return;
			}

			string? accessToken = (string?)tokenJson?["access_token"];
			if (string.IsNullOrEmpty(accessToken))
			{
				Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = "No access token in EA response" });
				return;
			}

			// send EA JWT to cypress master for cypress session
			var body = new JObject { ["token"] = accessToken };
			var content = new StringContent(body.ToString(), Encoding.UTF8, "application/json");
			var resp = await s_httpClient.PostAsync(MASTER_SERVER_URL + "/auth/login", content);
			string respRaw = await resp.Content.ReadAsStringAsync();
			JObject? respBody = null;
			try { respBody = JObject.Parse(respRaw); } catch { }

			if (!resp.IsSuccessStatusCode)
			{
				string err = (string?)respBody?["error"] ?? (respRaw.Length > 200 ? respRaw.Substring(0, 200) : respRaw);
				Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = $"Login failed: {err}" });
				return;
			}

			m_authToken = (string?)respBody?["token"];
			m_authPid = (string?)respBody?["pid"];
			m_authUid = (string?)respBody?["uid"]?.ToString();
			m_authDisplayName = (string?)respBody?["displayName"];
			m_authExpiresAt = (double)(respBody?["expiresAt"] ?? 0);
			m_eaAccessToken = accessToken;
			SaveAuthToDisk();

			Send(new JObject { ["type"] = "authLoginResult", ["ok"] = true, ["displayName"] = m_authDisplayName, ["pid"] = m_authPid, ["uid"] = m_authUid });

			// auto-register identity if not already done
			await AutoRegisterIdentityAsync();
		}
		catch (Exception ex)
		{
			Send(new JObject { ["type"] = "authLoginResult", ["ok"] = false, ["error"] = ex.Message });
		}
	}

	private void OnEaLogout()
	{
		if (m_authToken != null)
		{
			string token = m_authToken;
			Task.Run(async () =>
			{
				try
				{
					var req = new HttpRequestMessage(HttpMethod.Post, MASTER_SERVER_URL + "/auth/logout");
					req.Headers.Add("Authorization", "Bearer " + token);
					await s_httpClient.SendAsync(req);
				}
				catch { }
			});
		}

		ClearAuth();
		Send(new JObject { ["type"] = "authLogoutResult", ["ok"] = true });
	}

	private void OnGetAuthTicket()
	{
		if (m_authToken == null)
		{
			Send(new JObject { ["type"] = "authTicket", ["ok"] = false, ["error"] = "Not logged in" });
			return;
		}

		Task.Run(async () =>
		{
			try
			{
				var req = new HttpRequestMessage(HttpMethod.Post, MASTER_SERVER_URL + "/auth/ticket");
				req.Headers.Add("Authorization", "Bearer " + m_authToken);
				var resp = await s_httpClient.SendAsync(req);
				var body = JObject.Parse(await resp.Content.ReadAsStringAsync());

				if (resp.IsSuccessStatusCode)
				{
					Send(new JObject { ["type"] = "authTicket", ["ok"] = true, ["ticket"] = (string?)body["ticket"] });
				}
				else
				{
					Send(new JObject { ["type"] = "authTicket", ["ok"] = false, ["error"] = (string?)body["error"] ?? "Failed to get ticket" });
				}
			}
			catch (Exception ex)
			{
				Send(new JObject { ["type"] = "authTicket", ["ok"] = false, ["error"] = ex.Message });
			}
		});
	}

	// fetch owned PvZ games from EA entitlements using the in-memory EA token
	internal async Task<List<string>> FetchOwnedGamesFromEAAsync()
	{
		if (string.IsNullOrEmpty(m_eaAccessToken)) return new List<string>();

		var groupToName = new Dictionary<string, string>
		{
			["PVZGWPC"]  = "Garden Warfare 1",
			["PVZGW2PC"] = "Garden Warfare 2",
			["PVZGW3PC"] = "Battle for Neighborville"
		};

		try
		{
			var req = new HttpRequestMessage(HttpMethod.Get,
				"https://gateway.ea.com/proxy/identity/pids/me/entitlements?expand=entitlement");
			req.Headers.Add("Authorization", "Bearer " + m_eaAccessToken);
			var resp = await s_httpClient.SendAsync(req);
			if (!resp.IsSuccessStatusCode) return new List<string>();

			var body = JObject.Parse(await resp.Content.ReadAsStringAsync());

			// collect full entitlement objects (may be empty if API returned URIs instead)
			var entries = (body["entitlements"]?["entitlement"] as JArray) ?? new JArray();

			// fallback: if only URIs were returned, fetch each one individually
			if (entries.Count == 0)
			{
				var uris = body["entitlements"]?["entitlementUri"] as JArray;
				if (uris != null)
				{
					foreach (var uriToken in uris)
					{
						string uri = (string?)uriToken ?? "";
						if (string.IsNullOrEmpty(uri)) continue;
						try
						{
							var ereq = new HttpRequestMessage(HttpMethod.Get,
								"https://gateway.ea.com/proxy/identity" + uri);
							ereq.Headers.Add("Authorization", "Bearer " + m_eaAccessToken);
							var eresp = await s_httpClient.SendAsync(ereq);
							if (!eresp.IsSuccessStatusCode) continue;
							var ebody = JObject.Parse(await eresp.Content.ReadAsStringAsync());
							var single = ebody["entitlement"] as JObject;
							if (single != null) entries.Add(single);
						}
						catch { }
					}
				}
			}

			var owned = new HashSet<string>();
			foreach (var e in entries)
			{
				string group  = (string?)e["groupName"]        ?? "";
				string type   = (string?)e["entitlementType"]  ?? "";
				string status = (string?)e["status"]           ?? "";
				if (groupToName.TryGetValue(group, out var name) && type == "ONLINE_ACCESS" && status == "ACTIVE")
					owned.Add(name);
			}
			return owned.ToList();
		}
		catch { return new List<string>(); }
	}

	private void OnRefreshEntitlements()
	{
		if (string.IsNullOrEmpty(m_authToken))
		{
			Send(new JObject { ["type"] = "refreshEntitlementsResult", ["ok"] = false, ["error"] = "Not logged in" });
			return;
		}
		if (string.IsNullOrEmpty(m_eaAccessToken))
		{
			Send(new JObject { ["type"] = "refreshEntitlementsResult", ["ok"] = false, ["error"] = "EA session expired - please re-sign in with EA to refresh" });
			return;
		}

		Task.Run(async () =>
		{
			try
			{
				var body = new JObject { ["ea_token"] = m_eaAccessToken };
				var req = new HttpRequestMessage(HttpMethod.Post, MASTER_SERVER_URL + "/auth/refresh-entitlements")
				{
					Content = new StringContent(body.ToString(), Encoding.UTF8, "application/json")
				};
				req.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", m_authToken);

				var resp = await s_httpClient.SendAsync(req);
				var respBody = JObject.Parse(await resp.Content.ReadAsStringAsync());

				if (resp.IsSuccessStatusCode && (bool)(respBody["ok"] ?? false))
				{
					string? newJwt = (string?)respBody["jwt"];
					if (!string.IsNullOrEmpty(newJwt))
					{
						m_identityJwt = newJwt;
						m_identityExpiresAt = ParseJwtExpiry(m_identityJwt);
						ParseJwtIdentity(m_identityJwt);
						SaveIdentityToDisk();
					}
					var ownedGames = BuildOwnedGamesList();
					Send(new JObject
					{
						["type"] = "refreshEntitlementsResult",
						["ok"] = true,
						["ownedGames"] = new JArray(ownedGames)
					});
				}
				else
				{
					Send(new JObject { ["type"] = "refreshEntitlementsResult", ["ok"] = false, ["error"] = (string?)respBody["error"] ?? "Failed" });
				}
			}
			catch (Exception ex)
			{
				Send(new JObject { ["type"] = "refreshEntitlementsResult", ["ok"] = false, ["error"] = ex.Message });
			}
		});
	}

	private void OnRelinkEA()
	{
		if (m_eaRelinked)
		{
			Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = "Already relinked once" });
			return;
		}
		if (m_identityJwt == null || m_authToken == null)
		{
			Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = "Not logged in" });
			return;
		}

		CleanupLegacyQrcHandler();

		var verifierBytes = new byte[32];
		RandomNumberGenerator.Fill(verifierBytes);
		m_codeVerifier = Base64UrlEncode(verifierBytes);
		byte[] challengeHash = SHA256.HashData(Encoding.ASCII.GetBytes(m_codeVerifier));
		string codeChallenge = Base64UrlEncode(challengeHash);
		string pcSign = GeneratePcSign();
		string authUrl = $"{EA_AUTH_URL}?client_id={EA_CLIENT_ID}"
			+ $"&response_type=code"
			+ $"&redirect_uri={Uri.EscapeDataString(EA_REDIRECT_URI)}"
			+ $"&code_challenge={codeChallenge}"
			+ $"&code_challenge_method=S256"
			+ $"&pc_sign={Uri.EscapeDataString(pcSign)}";

#if WINDOWS
		Task.Run(async () =>
		{
			try
			{
				var result = await EaLoginWindow.ShowAsync(authUrl, EA_REDIRECT_URI);
				if (result.Cancelled)
				{
					Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = "Login cancelled" });
					return;
				}
				if (!string.IsNullOrEmpty(result.Error))
				{
					Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = result.Error });
					return;
				}
				await DoRelinkAsync(result.Code!);
			}
			catch (Exception ex)
			{
				Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = ex.Message });
			}
		});
#else
		Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = "Not supported on this platform" });
#endif
	}

	private async Task DoRelinkAsync(string code)
	{
		//don't update main auth session
		var tokenBody = new FormUrlEncodedContent(new[]
		{
			new KeyValuePair<string, string>("grant_type", "authorization_code"),
			new KeyValuePair<string, string>("code", code),
			new KeyValuePair<string, string>("client_id", EA_CLIENT_ID),
			new KeyValuePair<string, string>("client_secret", EA_CLIENT_SECRET),
			new KeyValuePair<string, string>("code_verifier", m_codeVerifier!),
			new KeyValuePair<string, string>("redirect_uri", EA_REDIRECT_URI),
			new KeyValuePair<string, string>("token_format", "JWS"),
		});

		var tokenResp = await s_httpClient.PostAsync(EA_TOKEN_URL, tokenBody);
		var tokenRaw = await tokenResp.Content.ReadAsStringAsync();
		JObject? tokenJson = null;
		try { tokenJson = JObject.Parse(tokenRaw); } catch { }

		if (!tokenResp.IsSuccessStatusCode)
		{
			string err = (string?)tokenJson?["error_description"] ?? tokenRaw;
			Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = $"EA token exchange failed: {err}" });
			return;
		}

		string? newEAToken = (string?)tokenJson?["access_token"];
		if (string.IsNullOrEmpty(newEAToken))
		{
			Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = "No access token" });
			return;
		}

		// call relink endpoint
		m_identityKey ??= LoadOrCreateIdentityKey();
		byte[] challenge = SHA256.HashData(Encoding.UTF8.GetBytes(m_identityJwt!));
		string challengeSig = SignChallenge(challenge);

		var body = new JObject
		{
			["jwt"]           = m_identityJwt,
			["challenge_sig"] = challengeSig,
			["ea_token"]      = newEAToken
		};
		var req = new HttpRequestMessage(HttpMethod.Post, MASTER_SERVER_URL + "/auth/relink-ea")
		{
			Content = new StringContent(body.ToString(), Encoding.UTF8, "application/json")
		};
		req.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", m_authToken);

		var resp = await s_httpClient.SendAsync(req);
		var respBody = JObject.Parse(await resp.Content.ReadAsStringAsync());

		if (!resp.IsSuccessStatusCode)
		{
			Send(new JObject { ["type"] = "relinkEAResult", ["ok"] = false, ["error"] = (string?)respBody["error"] ?? "Relink failed" });
			return;
		}

		m_identityJwt = (string?)respBody["jwt"] ?? m_identityJwt;
		m_identityExpiresAt = ParseJwtExpiry(m_identityJwt);
		ParseJwtIdentity(m_identityJwt);
		m_eaRelinked = true;
		m_eaAccessToken = newEAToken;
		SaveIdentityToDisk();

		var ownedGames = BuildOwnedGamesList();
		Send(new JObject
		{
			["type"]     = "relinkEAResult",
			["ok"]       = true,
			["eaName"]   = (string?)respBody["eaName"],
			["ownedGames"] = new JArray(ownedGames)
		});
	}

	private void ClearAuth()
	{
		m_authToken = null;
		m_authPid = null;
		m_authDisplayName = null;
		m_authExpiresAt = 0;
		m_eaAccessToken = null;
		SaveAuthToDisk();
	}

	private void LoadAuthFromDisk()
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			if (!File.Exists(filePath)) return;
			var root = JObject.Parse(File.ReadAllText(filePath));
			var auth = root["Auth"] as JObject;
			if (auth == null) return;
			m_authToken = (string?)auth["Token"];
			m_authPid = (string?)auth["PID"];
			m_authUid = (string?)auth["UID"];
			m_authDisplayName = (string?)auth["DisplayName"];
			m_authExpiresAt = (double)(auth["ExpiresAt"] ?? 0);
		}
		catch { }
	}

	private void SaveAuthToDisk()
	{
		try
		{
			string filePath = Path.Combine(GetAppdataDir(), s_launcherSavedataFilename);
			JObject root = new JObject();
			if (File.Exists(filePath))
				root = JObject.Parse(File.ReadAllText(filePath));

			if (m_authToken != null)
			{
				root["Auth"] = new JObject
				{
					["Token"] = m_authToken,
					["PID"] = m_authPid,
					["UID"] = m_authUid,
					["DisplayName"] = m_authDisplayName,
					["ExpiresAt"] = m_authExpiresAt
				};
			}
			else
			{
				root.Remove("Auth");
			}

			File.WriteAllText(filePath, root.ToString());
		}
		catch { }
	}

	private static readonly byte[] s_pcSignKeyV1 = Encoding.ASCII.GetBytes("ISa3dpGOc8wW7Adn4auACSQmaccrOyR2");
	private static readonly byte[] s_pcSignKeyV2 = Encoding.ASCII.GetBytes("nt5FfJbdPzNcl2pkC3zgjO43Knvscxft");

	private static string GeneratePcSign()
	{
		// pick a random signing key version
		bool useV2 = RandomNumberGenerator.GetInt32(2) == 1;
		byte[] signKey = useV2 ? s_pcSignKeyV2 : s_pcSignKeyV1;
		string sv = useV2 ? "v2" : "v1";

		// gather basic machine identifiers
		string machineName = Environment.MachineName;
		string ts = DateTime.UtcNow.ToString("yyyy-MM-dd HH:mm:ss:fff");

		// fnv1a of machine name as mid
		ulong mid = Fnv1a(Encoding.UTF8.GetBytes(machineName));

		var payload = new JObject
		{
			["av"] = "v1",
			["bsn"] = machineName,
			["gid"] = 0,
			["hsn"] = "None",
			["mid"] = mid.ToString(),
			["msn"] = machineName,
			["sv"] = sv,
			["ts"] = ts
		};

		string payloadJson = payload.ToString(Newtonsoft.Json.Formatting.None);
		string payloadB64 = Base64UrlEncode(Encoding.UTF8.GetBytes(payloadJson));

		using var hmac = new HMACSHA256(signKey);
		byte[] sig = hmac.ComputeHash(Encoding.UTF8.GetBytes(payloadB64));
		string sigB64 = Base64UrlEncode(sig);

		return payloadB64 + "." + sigB64;
	}

	private static ulong Fnv1a(byte[] data)
	{
		ulong hash = 14695981039346656037;
		foreach (byte b in data)
		{
			hash ^= b;
			hash *= 1099511628211;
		}
		return hash;
	}

	private static string Base64UrlEncode(byte[] data)
	{
		return Convert.ToBase64String(data)
			.TrimEnd('=')
			.Replace('+', '-')
			.Replace('/', '_');
	}
}
