using System.IO;
using System.Text.Json;
using Microsoft.Win32;

namespace Teldrive.Desktop;

public sealed record DesktopPreferences
{
    public string Language { get; init; } = "es";
    public bool AlwaysShowInTray { get; init; }
    public bool MinimizeToTray { get; init; }
    public bool CloseToTray { get; init; }
    public bool StartInTray { get; init; }
    public bool StartWithWindows { get; init; }
    public bool NotifyFinished { get; init; } = true;

    public static DesktopPreferences Load(string root)
    {
        var path = Path.Combine(root, "desktop-preferences.json");
        return File.Exists(path) ? JsonSerializer.Deserialize<DesktopPreferences>(File.ReadAllText(path)) ?? throw new IOException(L.T("El archivo de preferencias está vacío.")) : new();
    }
    public void Save(string root)
    {
        var path = Path.Combine(root, "desktop-preferences.json");
        File.WriteAllText(path + ".new", JsonSerializer.Serialize(this, new JsonSerializerOptions { WriteIndented = true }));
        File.Move(path + ".new", path, true);
    }
}

internal static class DesktopStartup
{
    private const string ValueName = "TeldriveDesktopV2";
    public static string Command(string executable, string root, bool portable)
    {
        if (executable.Contains('"') || root.Contains('"') || !Path.IsPathFullyQualified(executable) || !Path.IsPathFullyQualified(root)) throw new ArgumentException(L.T("La ubicación del programa no es válida para el inicio automático."));
        var startupRoot = Path.TrimEndingDirectorySeparator(root);
        if (startupRoot.EndsWith('\\')) startupRoot += "\\"; // Windows argv quotes require doubling a final backslash.
        return $"\"{executable}\" --start-in-tray " + (portable ? "--portable" : $"--data-root \"{startupRoot}\"");
    }
    // Only called after saving the user-selected preference. Smoke tests never write Run.
    public static void Apply(bool enabled, string root, bool portable)
    {
        using var key = Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Run", true);
        if (enabled) key.SetValue(ValueName, Command(Environment.ProcessPath ?? throw new IOException(L.T("No se pudo localizar el ejecutable.")), root, portable));
        else key.DeleteValue(ValueName, false);
    }
}
