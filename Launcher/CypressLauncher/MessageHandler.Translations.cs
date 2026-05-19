#nullable enable
using System;
using System.IO;
using System.Linq;
using Newtonsoft.Json.Linq;

namespace CypressLauncher;

public partial class MessageHandler
{
    private static string GetTranslationsDir() =>
        Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData), "Cypress", "Translations");

    private void OnGetTranslations(string lang)
    {
        if (string.IsNullOrWhiteSpace(lang) || !System.Text.RegularExpressions.Regex.IsMatch(lang, @"^[a-zA-Z0-9_\-]{1,32}$"))
            lang = "en_us";

        string appdataDir = GetTranslationsDir();
        string appdataFile = Path.Combine(appdataDir, lang + ".json");

        string bundledFile = Path.Combine(AppContext.BaseDirectory, "assets", "translations", lang + ".json");
        JObject? bundled = null;
        if (File.Exists(bundledFile))
        {
            try { bundled = JObject.Parse(File.ReadAllText(bundledFile)); } catch { }
        }

        if (File.Exists(appdataFile))
        {
            try
            {
                var userStrings = JObject.Parse(File.ReadAllText(appdataFile));
                if (bundled != null)
                {
                    foreach (var prop in bundled.Properties())
                    {
                        if (userStrings[prop.Name] == null)
                            userStrings[prop.Name] = prop.Value;
                    }
                }
                Send(new JObject { ["type"] = "translations", ["lang"] = lang, ["strings"] = userStrings });
                return;
            }
            catch { }
        }

        if (bundled != null)
        {
            // seed AppData with bundled file so the user can customize it later
            try
            {
                Directory.CreateDirectory(appdataDir);
                if (!File.Exists(appdataFile))
                    File.Copy(bundledFile, appdataFile);
            }
            catch { }

            Send(new JObject { ["type"] = "translations", ["lang"] = lang, ["strings"] = bundled });
            return;
        }

        if (lang != "en_us")
        {
            OnGetTranslations("en_us");
            return;
        }

        Send(new JObject { ["type"] = "translations", ["lang"] = "en_us", ["strings"] = new JObject() });
    }

    private void OnGetTranslationsList()
    {
        var langs = new System.Collections.Generic.HashSet<string>();

        string bundledDir = Path.Combine(AppContext.BaseDirectory, "assets", "translations");
        if (Directory.Exists(bundledDir))
        {
            foreach (string f in Directory.GetFiles(bundledDir, "*.json"))
                langs.Add(Path.GetFileNameWithoutExtension(f));
        }

        string appdataDir = GetTranslationsDir();
        if (Directory.Exists(appdataDir))
        {
            foreach (string f in Directory.GetFiles(appdataDir, "*.json"))
                langs.Add(Path.GetFileNameWithoutExtension(f));
        }

        Send(new JObject { ["type"] = "translationsList", ["langs"] = new JArray(langs.OrderBy(x => x)) });
    }
}
