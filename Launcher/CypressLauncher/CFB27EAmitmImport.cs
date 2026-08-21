#nullable enable
using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Threading.Tasks;

namespace CypressLauncher;

/// <summary>
/// Imports the newest user-produced EA-MITM ACP file through the redacting,
/// read-only cfb27capture analyzer. This never loads a DLL or touches a game
/// process; it only consumes a completed file on disk.
/// </summary>
internal sealed class CFB27EAmitmImport
{
	private readonly Func<string> _appDataRoot;

	public CFB27EAmitmImport(Func<string> appDataRoot) => _appDataRoot = appDataRoot;

	public string EvidenceRoot => Path.Combine(_appDataRoot(), "Diagnostics", "CFB27", "ea-mitm");

	public async Task<EAmitmImportResult> ImportLatestAsync(string cypressRoot)
	{
		string? source = FindNewestSource();
		if (source == null)
			return EAmitmImportResult.Fail("No completed .acp file was found. Finish the user-operated capture first.");

		if (!await IsStableAsync(source).ConfigureAwait(false))
			return EAmitmImportResult.Fail("The newest .acp file is still being written; try again after capture stops.");

		string analyzer = Path.Combine(cypressRoot, "tools", "cypress-servers", "cfb27capture.exe");
		if (!File.Exists(analyzer))
			analyzer = Path.Combine(cypressRoot, "tools", "cypress-servers", "build", "cfb27capture.exe");
		if (!File.Exists(analyzer))
			return EAmitmImportResult.Fail("The bundled cfb27capture analyzer is missing.");

		Directory.CreateDirectory(EvidenceRoot);
		string report = Path.Combine(EvidenceRoot, Path.GetFileNameWithoutExtension(source) + "-redacted.json");
		var psi = new ProcessStartInfo
		{
			FileName = analyzer,
			UseShellExecute = false,
			RedirectStandardOutput = true,
			RedirectStandardError = true,
			CreateNoWindow = true
		};
		psi.ArgumentList.Add("-format");
		psi.ArgumentList.Add("json");
		psi.ArgumentList.Add(source);
		using var process = Process.Start(psi);
		if (process == null)
			return EAmitmImportResult.Fail("Could not start the cfb27capture analyzer.");
		string json = await process.StandardOutput.ReadToEndAsync().ConfigureAwait(false);
		string error = await process.StandardError.ReadToEndAsync().ConfigureAwait(false);
		await process.WaitForExitAsync().ConfigureAwait(false);
		if (process.ExitCode != 0)
			return EAmitmImportResult.Fail(string.IsNullOrWhiteSpace(error) ? "The analyzer failed." : error.Trim());

		File.WriteAllText(report, json);
		return new EAmitmImportResult(true, source, report, json.Length, null);
	}

	private static string? FindNewestSource()
	{
		string downloads = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
		string[] roots =
		{
			@"D:\Downloads\EA-MITM\out",
			Path.Combine(downloads, "Downloads", "EA-MITM", "out"),
			Path.Combine(downloads, "Downloads", "EA-MITM"),
			Path.Combine(downloads, "EA-MITM", "out")
		};
		return roots.Where(Directory.Exists)
			.SelectMany(r => Directory.EnumerateFiles(r, "*.acp", SearchOption.TopDirectoryOnly))
			.Select(p => new FileInfo(p))
			.OrderByDescending(f => f.LastWriteTimeUtc)
			.Select(f => f.FullName)
			.FirstOrDefault();
	}

	private static async Task<bool> IsStableAsync(string path)
	{
		long first = new FileInfo(path).Length;
		await Task.Delay(250).ConfigureAwait(false);
		return new FileInfo(path).Length == first;
	}
}

internal sealed record EAmitmImportResult(bool Ok, string? Source, string? Report, int Bytes, string? Error)
{
	public static EAmitmImportResult Fail(string error) => new(false, null, null, 0, error);
}
