#nullable enable
using System;
using System.IO;

namespace CypressLauncher;

internal static class PatchCli
{
	public static int Run(string[] args)
	{
		if (args.Length == 0)
		{
			PrintUsage();
			return 1;
		}

		string command = args[0].Trim().ToLowerInvariant();
		if (command is "--help" or "-h" or "help")
		{
			PrintUsage();
			return 0;
		}

		if (args.Length < 3)
		{
			PrintUsage();
			return 1;
		}

		if (!PatchManager.TryParseGame(args[1], out MessageHandler.PVZGame game))
		{
			Console.Error.WriteLine("unknown game: " + args[1]);
			PrintUsage();
			return 1;
		}

		string gameDirectory = args[2];
		string sourceExeName = MessageHandler.s_gameToExecutableName[game];

		void Log(string text, string level)
		{
			string prefix = level.ToLowerInvariant() switch
			{
				"error" => "[error]",
				"success" => "[ ok ]",
				_ => "[info]"
			};
			Console.WriteLine(prefix + " " + text);
		}

		switch (command)
		{
		case "patch":
		case "--patch":
			if (!MessageHandler.s_gameToPatchedExecutableName.TryGetValue(game, out string? patchedExeName))
			{
				Console.Error.WriteLine("game does not require patching: " + game);
				return 1;
			}
			return PatchManager.EnsurePatched(game, gameDirectory, sourceExeName, patchedExeName, Log) ? 0 : 1;
		case "restore":
		case "--restore":
			if (MessageHandler.s_gameToPatchedExecutableName.TryGetValue(game, out string? pe))
			{
				string patchedPath = Path.Combine(gameDirectory, pe);
				try
				{
					if (File.Exists(patchedPath))
						File.Delete(patchedPath);
					Console.WriteLine("[ ok ] deleted " + pe);
				}
				catch (Exception ex)
				{
					Console.Error.WriteLine("[error] " + ex.Message);
					return 1;
				}
			}
			else
			{
				Console.WriteLine("[info] game does not use a patched exe, nothing to restore");
			}
			return 0;
		case "status":
		case "--status":
			if (MessageHandler.s_gameToPatchedExecutableName.TryGetValue(game, out string? pexe))
				Console.WriteLine(File.Exists(Path.Combine(gameDirectory, pexe)) ? "patched" : "unpatched");
			else
				Console.WriteLine("no patch required");
			return 0;
		default:
			Console.Error.WriteLine("unknown command: " + args[0]);
			PrintUsage();
			return 1;
		}
	}

	private static void PrintUsage()
	{
		Console.WriteLine("cypress patch cli");
		Console.WriteLine("usage:");
		Console.WriteLine("  CypressLauncher patch <gw1|gw2|bfn> <game_dir>");
		Console.WriteLine("  CypressLauncher restore <gw1|gw2|bfn> <game_dir>");
		Console.WriteLine("  CypressLauncher status <gw1|gw2|bfn> <game_dir>");
	}
}
