using System.Globalization;
using System.IO;
using System.Text.RegularExpressions;

namespace Teldrive.Desktop;

public sealed record RcloneTaskDefinition
{
    public string Id { get; init; } = Guid.NewGuid().ToString("N");
    public string Name { get; init; } = L.T("Nueva tarea");
    public string Direction { get; init; } = "upload";
    public string Source { get; init; } = "";
    public string Destination { get; init; } = "/";
    public string Operation { get; init; } = "copy";
    public bool RunAtStartup { get; init; }
    public string MountCacheMode { get; init; } = "full";
    public string MountCacheSize { get; init; } = "10G";
    public string MountCacheAge { get; init; } = "24h";
    public bool MountReadOnly { get; init; }
    public bool IsMount => Operation == "mount";
    public string StartupDisplay => RunAtStartup ? L.T("Al iniciar") : L.T("Manual");
    public string RemoteName { get; init; } = "teldrive";
    public int Transfers { get; init; } = 4;
    public int Checkers { get; init; } = 8;
    public int Retries { get; init; } = 3;
    public int LowLevelRetries { get; init; } = 10;
    public int ConnectTimeout { get; init; } = 60;
    public int IdleTimeout { get; init; } = 300;
    public int MaxDepth { get; init; } = -1;
    public string Bandwidth { get; init; } = "";
    public string MinSize { get; init; } = "";
    public string MaxSize { get; init; } = "";
    public string MinAge { get; init; } = "";
    public string MaxAge { get; init; } = "";
    public string Compare { get; init; } = "default";
    public string DeleteMode { get; init; } = "after";
    public bool SkipExisting { get; init; }
    public bool SkipNewer { get; init; }
    public bool Immutable { get; init; } = true;
    public bool OneFileSystem { get; init; }
    public bool NoUpdateModTime { get; init; }
    public bool DeleteExcluded { get; init; }
    public bool AllowDestructive { get; init; }
    public string Includes { get; init; } = "";
    public string Excludes { get; init; } = "";
    public string Description => $"{Name} · {Operation} · {Source} → {Destination}";
    public string OperationDisplay => Operation switch { "mount" => L.T("Montar unidad"), "move" => L.T("Mover"), "sync" => L.T("Sincronizar"), "check" => L.T("Verificar"), _ => L.T("Copiar") };

    public string[] Arguments(bool dryRun, bool requireExistingSource = true)
    {
        if (string.IsNullOrWhiteSpace(Name) || Name.Length > 160 || Name.Contains('\0')) throw new ArgumentException(L.T("Asigna un nombre de hasta 160 caracteres a la tarea."));
        if (RunAtStartup && !IsMount) throw new ArgumentException(L.T("El montaje al iniciar solo se aplica a tareas de montaje de unidad."));
        if (IsMount) {
            var drive = Destination.Trim().ToUpperInvariant();
            if (!Regex.IsMatch(drive, "^[D-Z]:$")) throw new ArgumentException(L.T("Elige una letra libre entre D: y Z: como destino del montaje."));
            var remotePath = RcloneRuntime.Remote(Source, RemoteName);
            // A preview lists the remote; it never creates a Windows drive.
            if (dryRun) return ["lsf", remotePath, "--max-depth", "1"];
            if (MountCacheMode is not ("off" or "minimal" or "writes" or "full")) throw new ArgumentException(L.T("Modo de caché del montaje no válido."));
            if (!Regex.IsMatch(MountCacheSize, @"^\d+(?:\.\d+)?(?:[kKmMgGtT](?:i?[bB])?)?$" ) || !Regex.IsMatch(MountCacheAge, @"^\d+(?:\.\d+)?(?:s|m|h|d)$")) throw new ArgumentException(L.T("Revisa el límite y la duración de la caché. Ejemplos: 10G y 24h."));
            var mount = new List<string> { "mount", remotePath, drive, "--vfs-cache-mode", MountCacheMode, "--vfs-cache-max-size", MountCacheSize, "--vfs-cache-max-age", MountCacheAge, "--volname", L.T("Teldrive v2") };
            if (MountReadOnly) mount.Add("--read-only");
            return mount.ToArray();
        }
        if (Direction is not ("upload" or "download") || Operation is not ("copy" or "move" or "sync" or "check")) throw new ArgumentException(L.T("Selecciona una dirección y una operación válidas."));
        if (!dryRun && (Operation is "move" or "sync" || DeleteExcluded) && !AllowDestructive) throw new ArgumentException(L.T("Autoriza los cambios que eliminan archivos antes de ejecutar o programar esta tarea."));
        if (DeleteExcluded && Operation != "sync") throw new ArgumentException(L.T("Eliminar excluidos solo se aplica a sincronización."));
        if (Immutable && Operation is "move" or "sync") throw new ArgumentException(L.T("Desactiva «No sobrescribir archivos diferentes» para mover o sincronizar."));
        if (Transfers is < 1 or > 32 || Checkers is < 1 or > 64 || Retries is < 1 or > 20 || LowLevelRetries is < 1 or > 100 || ConnectTimeout is < 1 or > 3600 || IdleTimeout is < 1 or > 86400 || MaxDepth is < -1 or > 1000) throw new ArgumentException(L.T("Revisa los límites de transferencia, reintentos y tiempos de espera."));
        var local = Direction == "upload" ? Source : Destination;
        if (string.IsNullOrWhiteSpace(local) || !Path.IsPathFullyQualified(local) || local.Contains('\0')) throw new ArgumentException(L.T("Elige una ruta local absoluta mediante el selector de Windows."));
        var fullLocal = Path.GetFullPath(local);
        if (requireExistingSource && Direction == "upload" && !File.Exists(fullLocal) && !Directory.Exists(fullLocal)) throw new IOException(L.T("El origen local no está disponible."));
        if (Direction == "upload" && File.Exists(fullLocal) && Operation is "sync" or "check") throw new ArgumentException(L.T("Sincronizar y verificar requieren una carpeta como origen."));
        var remote = RcloneRuntime.Remote(Direction == "upload" ? Destination : Source, RemoteName);
        // copy/move take a destination folder; sync/check compare complete directory trees.
        var result = new List<string> { Operation, Direction == "upload" ? fullLocal : remote, Direction == "upload" ? remote : fullLocal, "--stats", "2s", "--stats-one-line", "--transfers", Transfers.ToString(CultureInfo.InvariantCulture), "--checkers", Checkers.ToString(CultureInfo.InvariantCulture), "--retries", Retries.ToString(CultureInfo.InvariantCulture), "--low-level-retries", LowLevelRetries.ToString(CultureInfo.InvariantCulture), "--contimeout", ConnectTimeout + "s", "--timeout", IdleTimeout + "s" };
        if (dryRun) result.Add("--dry-run");
        if (MaxDepth >= 0) result.AddRange(["--max-depth", MaxDepth.ToString(CultureInfo.InvariantCulture)]);
        AddQuantity("--bwlimit", Bandwidth, false); AddQuantity("--min-size", MinSize, false); AddQuantity("--max-size", MaxSize, false); AddQuantity("--min-age", MinAge, true); AddQuantity("--max-age", MaxAge, true);
        switch (Compare) {
            case "default": break;
            case "checksum": result.Add("--checksum"); break;
            case "checksum-only": result.AddRange(["--checksum", "--ignore-size"]); break;
            case "size": result.Add("--size-only"); break;
            case "modtime": result.Add("--ignore-size"); break;
            default: throw new ArgumentException(L.T("Comparación no válida."));
        }
        if (SkipExisting) result.Add("--ignore-existing");
        if (SkipNewer) result.Add("--update");
        if (Immutable && Operation != "check") result.Add("--immutable");
        if (OneFileSystem) result.Add("--one-file-system");
        if (NoUpdateModTime) result.Add("--no-update-modtime");
        if (Operation == "sync") {
            if (DeleteMode is not ("before" or "during" or "after")) throw new ArgumentException(L.T("Momento de eliminación no válido."));
            result.Add("--delete-" + DeleteMode);
            if (DeleteExcluded) result.Add("--delete-excluded");
        }
        AddPatterns("--include", Includes); AddPatterns("--exclude", Excludes);
        return result.ToArray();

        void AddQuantity(string flag, string value, bool duration) {
            if (string.IsNullOrWhiteSpace(value)) return;
            value = value.Trim();
            var pattern = duration ? @"^\d+(?:\.\d+)?(?:ms|s|m|h|d|w|M|y)?$" : @"^\d+(?:\.\d+)?(?:[bBkKmMgGtTpP](?:i?[bB])?)?$";
            if (!Regex.IsMatch(value, pattern, RegexOptions.CultureInvariant)) throw new ArgumentException(L.T("Valor no v\u00E1lido para {0}. Ejemplos: 10M, 1G, 24h.", flag));
            result.AddRange([flag, value]);
        }
        void AddPatterns(string flag, string value) {
            if (value.Length > 16000 || value.Contains('\0')) throw new ArgumentException(L.T("La lista de filtros es demasiado grande o contiene caracteres no válidos."));
            foreach (var line in value.Split('\n').Select(item => item.Trim()).Where(item => item.Length != 0)) result.AddRange([flag, line]);
        }
    }
}
