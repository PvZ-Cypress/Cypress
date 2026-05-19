#nullable enable
using System;
using System.IO;
using System.Reflection;

namespace CypressLauncher;

internal static class ResourceBuilder
{
	private static readonly string[] s_cssOrder =
	{
		"wwwroot.css.base.css",
		"wwwroot.css.titlebar.css",
		"wwwroot.css.layout.css",
		"wwwroot.css.sidebar.css",
		"wwwroot.css.profile.css",
		"wwwroot.css.forms.css",
		"wwwroot.css.motd.css",
		"wwwroot.css.content.css",
		"wwwroot.css.host.css",
		"wwwroot.css.docs.css",
		"wwwroot.css.sections.css",
		"wwwroot.css.pickers.css",
		"wwwroot.css.modifiers.css",
		"wwwroot.css.bfn.css",
		"wwwroot.css.playlist.css",
		"wwwroot.css.steps.css",
		"wwwroot.css.instances.css",
		"wwwroot.css.browser.css",
		"wwwroot.css.auth.css",
		"wwwroot.css.moderator.css",
	};

	private static readonly string[] s_jsOrder =
	{
		"wwwroot.js.i18n.js",
		"wwwroot.js.data.js",
		"wwwroot.js.auth.js",
		"wwwroot.js.core.js",
		"wwwroot.js.tabs.js",
		"wwwroot.js.helpers.js",
		"wwwroot.js.relay.js",
		"wwwroot.js.pickers.js",
		"wwwroot.js.levelmode.js",
		"wwwroot.js.modifiers.js",
		"wwwroot.js.settings.js",
		"wwwroot.js.profile.js",
		"wwwroot.js.actions.js",
		"wwwroot.js.motd.js",
		"wwwroot.js.serverinfo.js",
		"wwwroot.js.browser.js",
		"wwwroot.js.playlist.js",
		"wwwroot.js.instances.js",
		"wwwroot.js.moderator.js",
		"wwwroot.js.updates.js",
		"wwwroot.js.docs.js",
		"wwwroot.js.init.js",
	};

	public static string BuildHtml(Assembly assembly)
	{
		string html = ReadEmbeddedResource(assembly, "wwwroot.index.html");
		string css = ConcatEmbeddedResources(assembly, s_cssOrder);
		string js = ConcatEmbeddedResources(assembly, s_jsOrder);

		html = html.Replace("{{LOGO_BASE64}}", GetLogoBase64(256));
		html = html.Replace("{{GW1_ICON_BASE64}}", GetGameIconBase64("gw1_icon", 120));
		html = html.Replace("{{GW2_ICON_BASE64}}", GetGameIconBase64("gw2_icon", 120));
		html = html.Replace("{{BFN_ICON_BASE64}}", GetGameIconBase64("bfn_icon", 120));
		html = html.Replace("{{GW1_BG_BASE64}}", GetGameBgBase64("gw1_bg", 960));
		html = html.Replace("{{GW2_BG_BASE64}}", GetGameBgBase64("gw2_bg", 960));
		html = html.Replace("{{BFN_BG_BASE64}}", GetGameBgBase64("bfn_bg", 960));

		string fontFace = GetBurbankFontFace();
		css = fontFace + css;

		html = html.Replace("<link rel=\"stylesheet\" href=\"styles.css\">", "<style>" + css + "</style>");
		html = html.Replace("<script src=\"app.js\"></script>", "<script>" + js + "</script>");

		return html;
	}

	private static string GetLogoBase64(int size)
	{
		string dir = Path.Combine(AppContext.BaseDirectory, "assets", "cypressicons");
		foreach (string ext in new[] { "webp", "png" })
		{
			string path = Path.Combine(dir, ext, $"Burbank-CypressIcon.{ext}");
			if (File.Exists(path))
				return ImageHelper.ResizeToSquarePngBase64(path, size);
		}
		return string.Empty;
	}

	private static string GetGameIconBase64(string baseName, int maxHeight)
	{
		string dir = Path.Combine(AppContext.BaseDirectory, "assets", "gameicons");
		foreach (string ext in new[] { "webp", "png" })
		{
			string path = Path.Combine(dir, $"{baseName}.{ext}");
			if (File.Exists(path))
				return ImageHelper.ResizeByHeightToPngBase64(path, maxHeight);
		}
		return string.Empty;
	}

	private static string GetGameBgBase64(string baseName, int maxWidth)
	{
		string dir = Path.Combine(AppContext.BaseDirectory, "assets", "gamebgs");
		foreach (string ext in new[] { "webp", "jpg", "png" })
		{
			string path = Path.Combine(dir, $"{baseName}.{ext}");
			if (File.Exists(path))
				return ImageHelper.ResizeByWidthToJpegBase64(path, maxWidth, 90);
		}
		return string.Empty;
	}

	private static string GetBurbankFontFace()
	{
		string fontsDir = Path.Combine(AppContext.BaseDirectory, "assets", "fonts");
		string[] fontNames = { "burbankbigcondensed_bold.otf", "BurbankBigCondensed-Bold.otf", "BurbankBigCondensed-Bold.ttf", "Burbank.ttf", "Burbank.otf" };

		foreach (string fontName in fontNames)
		{
			string fontPath = Path.Combine(fontsDir, fontName);
			if (!File.Exists(fontPath))
				continue;

			try
			{
				byte[] fontBytes = File.ReadAllBytes(fontPath);
				string ext = Path.GetExtension(fontPath).ToLowerInvariant();
				string mime = ext == ".otf" ? "font/otf" : "font/ttf";
				string format = ext == ".otf" ? "opentype" : "truetype";
				string b64 = Convert.ToBase64String(fontBytes);
				return $"@font-face {{ font-family: 'Burbank'; src: url('data:{mime};base64,{b64}') format('{format}'); font-weight: 700; font-style: normal; }}\n";
			}
			catch { }
		}

		return string.Empty;
	}

	public static string ReadEmbeddedResource(Assembly assembly, string resourceSuffix)
	{
		string? fullName = null;
		foreach (string name in assembly.GetManifestResourceNames())
		{
			if (name.EndsWith(resourceSuffix, StringComparison.OrdinalIgnoreCase))
			{
				fullName = name;
				break;
			}
		}
		if (fullName == null)
			throw new FileNotFoundException("Embedded resource not found: " + resourceSuffix);

		using var stream = assembly.GetManifestResourceStream(fullName)!;
		using var reader = new StreamReader(stream);
		return reader.ReadToEnd();
	}

	private static string ConcatEmbeddedResources(Assembly assembly, string[] suffixes)
	{
		var sb = new System.Text.StringBuilder();
		foreach (string suffix in suffixes)
			sb.AppendLine(ReadEmbeddedResource(assembly, suffix));
		return sb.ToString();
	}
}
