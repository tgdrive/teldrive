using System.Text.RegularExpressions;

namespace Teldrive.Desktop;

public sealed record RcloneConfigOption(string Key, string Value)
{
    public bool Secret => Regex.IsMatch(Key, "pass|token|secret|key|credential|cookie", RegexOptions.IgnoreCase);
    public string DisplayValue => Secret && Value.Length > 0 ? "••••••••" : Value;
}

public static class RcloneConfigDocument
{
    private static readonly Regex Section = new(@"^\s*\[([^\]]+)\]\s*(?:[#;].*)?$");
    public static string[] Sections(string content) => Lines(content).Select(line => Section.Match(line)).Where(match => match.Success).Select(match => match.Groups[1].Value).Distinct().ToArray();
    public static RcloneConfigOption[] Options(string content, string section)
    {
        var current = ""; var values = new Dictionary<string, string>();
        foreach (var line in Lines(content)) {
            var heading = Section.Match(line);
            if (heading.Success) { current = heading.Groups[1].Value; continue; }
            if (current != section || line.TrimStart().StartsWith('#') || line.TrimStart().StartsWith(';')) continue;
            var split = line.IndexOf('=');
            if (split > 0) values[line[..split].Trim()] = line[(split + 1)..].Trim();
        }
        return values.Select(pair => new RcloneConfigOption(pair.Key, pair.Value)).ToArray();
    }
    public static string Set(string content, string section, string key, string? value)
    {
        if (string.IsNullOrWhiteSpace(section) || section.IndexOfAny(['[',']','\r','\n','\0']) >= 0 || !Regex.IsMatch(key, @"^[A-Za-z0-9_.-]+$") || value?.IndexOfAny(['\r','\n','\0']) >= 0) throw new ArgumentException(L.T("Revisa el nombre de conexión, la opción y su valor."));
        var lines = Lines(content).ToList();
        var start = lines.FindIndex(line => Section.Match(line) is { Success: true } match && match.Groups[1].Value == section);
        if (start < 0) { if (value is null) return content; lines.Add("[" + section + "]"); lines.Add(key + " = " + value); return string.Join('\n', lines) + "\n"; }
        var end = lines.FindIndex(start + 1, line => Section.IsMatch(line)); if (end < 0) end = lines.Count;
        var found = -1;
        for (var i = start + 1; i < end; i++) {
            var line = lines[i]; if (line.TrimStart().StartsWith('#') || line.TrimStart().StartsWith(';')) continue;
            var split = line.IndexOf('='); if (split > 0 && line[..split].Trim() == key) found = i;
        }
        if (found >= 0) { if (value is null) lines.RemoveAt(found); else lines[found] = key + " = " + value; }
        else if (value is not null) lines.Insert(end, key + " = " + value);
        return string.Join('\n', lines);
    }
    private static string[] Lines(string content) => content.Replace("\r\n", "\n").Split('\n');
}
