using System.Reflection;
using System.Text;
using CypressLauncher;

var gameDirectory = Path.Combine(Path.GetTempPath(), "cypress-cfb27-copy-test-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(gameDirectory);
var schemaTestRoot = Path.Combine(Path.GetTempPath(), "cypress-cfb27-schema-test-" + Guid.NewGuid().ToString("N"));
Directory.CreateDirectory(schemaTestRoot);

try
{
    var handler = new MessageHandler();
    var handlerType = typeof(MessageHandler);

    handlerType.GetField("m_selectedGame", BindingFlags.Instance | BindingFlags.NonPublic)!
        .SetValue(handler, MessageHandler.PVZGame.CFB27);
    handlerType.GetField("m_gameDirectory", BindingFlags.Instance | BindingFlags.NonPublic)!
        .SetValue(handler, gameDirectory);

    var copied = (bool)handlerType
        .GetMethod("CopyServerDLL", BindingFlags.Instance | BindingFlags.NonPublic)!
        .Invoke(handler, null)!;

    if (!copied)
        throw new InvalidOperationException("CopyServerDLL returned false for CFB27.");

    var packagedBridge = Path.Combine(AppContext.BaseDirectory, "cypress_CFB27.dll");
    var installedBridge = Path.Combine(gameDirectory, "dinput8.dll");
    if (!File.Exists(installedBridge))
        throw new InvalidOperationException("CFB27 launch did not install dinput8.dll.");
    if (!File.ReadAllBytes(packagedBridge).SequenceEqual(File.ReadAllBytes(installedBridge)))
        throw new InvalidOperationException("Installed dinput8.dll differs from the packaged CFB27 bridge.");
    if (!File.Exists(Path.Combine(gameDirectory, "cfb27-endpoints.json")))
        throw new InvalidOperationException("CFB27 launch did not install the endpoint manifest.");

    var servicesDirectory = Path.Combine(schemaTestRoot, "services");
    var dataDirectory = Path.Combine(schemaTestRoot, "data");
    var desktopDirectory = Path.Combine(schemaTestRoot, "Desktop");
    var releaseAssets0 = Path.Combine(desktopDirectory, "CFB27", "Release", "Dynasty_Assets", "0");
    var releaseAssets2 = Path.Combine(desktopDirectory, "CFB27", "Release", "Dynasty_Assets", "2");
    Directory.CreateDirectory(releaseAssets0);
    Directory.CreateDirectory(releaseAssets2);
    var selectSchemaRoot = handlerType.GetMethod("SelectCFB27SchemaRoot", BindingFlags.Static | BindingFlags.NonPublic)
        ?? throw new InvalidOperationException("SelectCFB27SchemaRoot is missing.");
    var selectedSchemaRoot = (string)selectSchemaRoot.Invoke(null, new object[] { servicesDirectory, dataDirectory, desktopDirectory })!;
    if (selectedSchemaRoot != releaseAssets0)
        throw new InvalidOperationException($"Launcher selected '{selectedSchemaRoot}' instead of authoritative slot-0 assets '{releaseAssets0}'.");

    var buildBlazeArguments = handlerType.GetMethod("BuildCFB27BlazeArguments", BindingFlags.Static | BindingFlags.NonPublic)
        ?? throw new InvalidOperationException("BuildCFB27BlazeArguments is missing.");
    var coachCatalog = Path.Combine(dataDirectory, "cfb27-assets-slot0.json");
    var blazeArguments = (string[])buildBlazeArguments.Invoke(null, new object[]
    {
        "LocalPlayer", Path.Combine(schemaTestRoot, "run"), coachCatalog
    })!;
    var catalogFlag = Array.IndexOf(blazeArguments, "-coach-catalog");
    if (catalogFlag < 0 || catalogFlag + 1 >= blazeArguments.Length || blazeArguments[catalogFlag + 1] != coachCatalog)
        throw new InvalidOperationException("Launcher did not pass the exported coach/team catalog to cfb27blaze.");

    var documentsDirectory = Path.Combine(schemaTestRoot, "Documents");
    var savesDirectory = Path.Combine(documentsDirectory, "EA SPORTS College Football 27", "Saves");
    Directory.CreateDirectory(savesDirectory);
    File.WriteAllText(Path.Combine(savesDirectory, "INVALID"), "not a dynasty");
    var dynastySeed = Path.Combine(savesDirectory, "DYNASTY-TEST");
    File.WriteAllBytes(dynastySeed, Encoding.ASCII.GetBytes("FBCHUNKS-test-seed"));
    var newerProfile = Path.Combine(savesDirectory, "PROFILE-COLLEGE");
    File.WriteAllBytes(newerProfile, Encoding.ASCII.GetBytes("FBCHUNKS-profile"));
    File.SetLastWriteTimeUtc(newerProfile, DateTime.UtcNow.AddMinutes(1));
    var selectDynastySeed = handlerType.GetMethod("SelectCFB27DynastySeed", BindingFlags.Static | BindingFlags.NonPublic)
        ?? throw new InvalidOperationException("SelectCFB27DynastySeed is missing.");
    var selectedSeed = (string)selectDynastySeed.Invoke(null, new object[] { documentsDirectory })!;
    if (selectedSeed != dynastySeed)
        throw new InvalidOperationException($"Launcher selected Dynasty seed '{selectedSeed}' instead of '{dynastySeed}'.");

    var buildDynastyArguments = handlerType.GetMethod("BuildCFB27DynastyArguments", BindingFlags.Static | BindingFlags.NonPublic)
        ?? throw new InvalidOperationException("BuildCFB27DynastyArguments is missing.");
    var franchiseTool = Path.Combine(servicesDirectory, "cmd", "cfb27franchise", "main.mjs");
    var dynastyArguments = (string[])buildDynastyArguments.Invoke(null, new object[]
    {
        selectedSchemaRoot, dataDirectory, servicesDirectory, selectedSeed, "node.exe"
    })!;
    foreach (var expected in new Dictionary<string, string>
    {
        ["-seed"] = selectedSeed,
        ["-data-dir"] = Path.Combine(dataDirectory, "dynasties"),
        ["-node"] = "node.exe",
        ["-franchise-tool"] = franchiseTool,
    })
    {
        var flag = Array.IndexOf(dynastyArguments, expected.Key);
        if (flag < 0 || flag + 1 >= dynastyArguments.Length || dynastyArguments[flag + 1] != expected.Value)
            throw new InvalidOperationException($"Launcher Dynasty arguments are missing {expected.Key} {expected.Value}.");
    }

    var validateDynastyHealth = handlerType.GetMethod("ValidateCFB27DynastyHealth", BindingFlags.Static | BindingFlags.NonPublic)
        ?? throw new InvalidOperationException("ValidateCFB27DynastyHealth is missing.");
    validateDynastyHealth.Invoke(null, new object[] { "{\"status\":\"ok\",\"artifactConfigured\":true}" });
    try
    {
        validateDynastyHealth.Invoke(null, new object[] { "{\"status\":\"ok\",\"artifactConfigured\":false}" });
        throw new InvalidOperationException("Launcher accepted a Dynasty backend without franchise persistence.");
    }
    catch (TargetInvocationException ex) when (ex.InnerException is InvalidOperationException)
    {
    }

    Console.WriteLine("CFB27 DLL staging, asset catalog, and persistent Dynasty wiring regression tests: PASS");
}
finally
{
    Directory.Delete(gameDirectory, recursive: true);
    Directory.Delete(schemaTestRoot, recursive: true);
}
