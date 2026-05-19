#nullable enable
using System;
using System.Diagnostics;
using System.IO;

namespace CypressLauncher;

public static class PatchManager
{
	public static bool IsWine()
	{
		if (Environment.GetEnvironmentVariable("WINELOADERNOEXEC") != null) return true;
		if (Environment.GetEnvironmentVariable("WINE_DIST") != null) return true;
		try
		{
			using var key = Microsoft.Win32.Registry.LocalMachine.OpenSubKey(@"SOFTWARE\Wine");
			if (key != null) return true;
		}
		catch { }
		return false;
	}

	public static bool EnsurePatched(
		MessageHandler.PVZGame game,
		string gameDirectory,
		string sourceExeName,
		string patchedExeName,
		Action<string, string> sendStatus)
	{
		string patchedPath = Path.Combine(gameDirectory, patchedExeName);
		if (File.Exists(patchedPath))
			return true;

		sendStatus("Creating patched executable (this might take a while)...", "info");

		string courgetteCmd = game == MessageHandler.PVZGame.BFN ? "-applybsdiff" : "-apply";
		bool wine = IsWine();
		var startInfo = new ProcessStartInfo
		{
			FileName = "courgette.exe",
			Arguments = $"{courgetteCmd} \"{Path.Combine(gameDirectory, sourceExeName)}\" {game}.patch \"{patchedPath}\"",
			Verb = wine ? "" : "runas",
			UseShellExecute = !wine,
			CreateNoWindow = wine,
			RedirectStandardOutput = wine,
			RedirectStandardError = wine
		};

		try
		{
			var process = Process.Start(startInfo);
			process?.WaitForExit();
			if (process?.ExitCode != 0)
			{
				sendStatus("Patcher failed (Code: " + process?.ExitCode.ToString("X") + ")", "error");
				return false;
			}
			return true;
		}
		catch (Exception ex)
		{
			sendStatus("Failed to start courgette: " + ex.Message, "error");
			return false;
		}
	}

	public static bool TryParseGame(string value, out MessageHandler.PVZGame game)
	{
		switch (value.Trim().ToLowerInvariant())
		{
		case "gw1":
		case "gardenwarfare":
		case "gardenwarfare1":
			game = MessageHandler.PVZGame.GW1;
			return true;
		case "gw2":
		case "gardenwarfare2":
			game = MessageHandler.PVZGame.GW2;
			return true;
		case "bfn":
		case "gw3":
		case "neighborville":
			game = MessageHandler.PVZGame.BFN;
			return true;
		default:
			game = default;
			return false;
		}
	}

	internal static bool SameFileContentsSafe(string leftPath, string rightPath)
	{
		try
		{
			return SameFileContents(leftPath, rightPath);
		}
		catch
		{
			return false;
		}
	}

	private static bool SameFileContents(string leftPath, string rightPath)
	{
		var leftInfo = new FileInfo(leftPath);
		var rightInfo = new FileInfo(rightPath);
		if (!leftInfo.Exists || !rightInfo.Exists || leftInfo.Length != rightInfo.Length)
			return false;

		using FileStream left = File.OpenRead(leftPath);
		using FileStream right = File.OpenRead(rightPath);
		for (int leftByte = left.ReadByte(), rightByte = right.ReadByte();
			leftByte != -1 && rightByte != -1;
			leftByte = left.ReadByte(), rightByte = right.ReadByte())
		{
			if (leftByte != rightByte)
				return false;
		}

		return true;
	}
}
