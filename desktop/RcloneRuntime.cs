using System.Diagnostics;
using System.IO;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Net;
using System.Net.Sockets;

namespace Teldrive.Desktop;

public sealed record RcloneConnection(string ApiKey, int ChunkMiB = 512, int Concurrency = 4, bool Encrypt = false, bool Hash = true, string LinkPassword = "");
public sealed class RcloneRuntime : IAsyncDisposable, IRcloneExecutor
{
    private readonly RuntimeSession runtime;
    private readonly WindowsJob owner = new();
    private readonly Dictionary<string, RcloneTask> tasks = [];
    private readonly object sync = new();
    private readonly SemaphoreSlim configuration = new(1, 1);
    private string? password;
    public bool Encrypted => File.Exists(ConfigPath) && File.ReadLines(ConfigPath).Take(5).Any(line => line == "RCLONE_ENCRYPT_V0:");
    public bool Locked => Encrypted && password is null;
    public Func<bool>? AutomationReady { get; set; }
    public string? BlockReason => AutomationReady is not null && !AutomationReady() ? L.T("Inicia el servidor para continuar la cola.") : !File.Exists(ConfigPath) ? L.T("Guarda la conexión en Configuración → Rclone.") : Locked ? L.T("Desbloquea la configuración de rclone para continuar la cola.") : null;
    public int AvailableSlots { get { lock (sync) return Math.Max(0, 3 - tasks.Values.Count(task => !task.Mount && !task.Process.HasExited)); } }
    public string StartDefinition(RcloneTaskDefinition definition, bool dryRun)
    {
        var arguments = definition.Arguments(dryRun);
        if (!definition.IsMount || dryRun) return Start(definition.Name, false, arguments);
        if (!DriverInstalled) throw new IOException(L.T("Prepara WinFsp desde este panel antes de montar la unidad."));
        var drive = definition.Destination.Trim().ToUpperInvariant();
        lock (sync) {
            if (DriveInfo.GetDrives().Any(item => item.Name.StartsWith(drive, StringComparison.OrdinalIgnoreCase)) || tasks.Values.Any(task => task.Mount && !task.Process.HasExited && task.Arguments[2].Equals(drive, StringComparison.OrdinalIgnoreCase))) throw new IOException(L.T("Esa letra de unidad ya está en uso."));
            return Start(definition.Name, true, [.. arguments, "--cache-dir", Path.Combine(runtime.DataRoot, "cache", "rclone", drive[..1])]);
        }
    }
    public RcloneJobStatus? Snapshot(string id)
    {
        lock (sync) {
            if (!tasks.TryGetValue(id, out var task)) return null;
            return new(task.Process.HasExited ? task.Cancelled ? "cancelled" : task.Process.ExitCode == 0 ? "completed" : "failed" : "running", task.Log, task.Process.HasExited ? task.Process.ExitCode : null);
        }
    }
    public RcloneRuntime(RuntimeSession runtime) { this.runtime = runtime; }
    private string ConfigPath => Path.Combine(runtime.DataRoot, "rclone.conf");
    public string ConfigFilePath => ConfigPath;
    public bool DriverInstalled => File.Exists(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86), "WinFsp", "bin", "winfsp-x64.dll"));

    public async Task ConfigureAsync(RcloneConnection connection)
    {
        await configuration.WaitAsync();
        try {
        if (Locked) throw new IOException(L.T("Desbloquea la configuración antes de modificar la conexión."));
        if (connection.ApiKey.Length < 8 || (connection.ApiKey + connection.LinkPassword).IndexOfAny(['\r', '\n', '\0']) >= 0) throw new ArgumentException(L.T("Introduce una clave de API válida."));
        if (connection.ChunkMiB is < 64 or > 2000 || connection.ChunkMiB % 16 != 0 || connection.Concurrency is < 1 or > 32) throw new ArgumentException(L.T("Revisa el tamaño de fragmento y la concurrencia."));
        lock (sync) { if (tasks.Values.Any(task => !task.Process.HasExited)) throw new IOException(L.T("Detén las operaciones de rclone antes de cambiar su conexión.")); }
        var config = File.Exists(ConfigPath) ? Encrypted ? await ExecuteAsync(CommandFor(ConfigPath, password, "config", "show")) : await File.ReadAllTextAsync(ConfigPath) : "";
        var options = new Dictionary<string, string> { ["type"] = "teldrive", ["api_host"] = runtime.ServerUri!.GetLeftPart(UriPartial.Authority), ["api_key"] = connection.ApiKey, ["chunk_size"] = connection.ChunkMiB + L.T("Mi"), ["upload_concurrency"] = connection.Concurrency.ToString(), ["encrypt_files"] = connection.Encrypt.ToString().ToLowerInvariant(), ["hash_enabled"] = connection.Hash.ToString().ToLowerInvariant(), ["page_size"] = "200" };
        foreach (var option in options) config = RcloneConfigDocument.Set(config, "teldrive", option.Key, option.Value);
        config = RcloneConfigDocument.Set(config, "teldrive", "link_password", string.IsNullOrEmpty(connection.LinkPassword) ? null : connection.LinkPassword);
        var temp = ConfigPath + ".new";
        try {
            await File.WriteAllTextAsync(temp, config);
            if (Encrypted) await EncryptFileAsync(temp, password!);
            File.Move(temp, ConfigPath, true);
        } finally { File.Delete(temp); File.Delete(temp + ".old"); }
        } finally { configuration.Release(); }
    }

    public object Info() => new { configured = File.Exists(ConfigPath), encrypted = Encrypted, locked = Locked, driverInstalled = DriverInstalled, platform = "windows", version = "1.76.0-teldrive-v2.1" };
    public async Task<string> ReadConfigAsync()
    {
        await configuration.WaitAsync();
        try {
            if (!File.Exists(ConfigPath)) return "";
            if (Locked) throw new IOException(L.T("Desbloquea rclone para editar su configuración."));
            if (!Encrypted) return await File.ReadAllTextAsync(ConfigPath);
            return await ExecuteAsync(CommandFor(ConfigPath, password, "config", "show"));
        } finally { configuration.Release(); }
    }
    public async Task SaveConfigAsync(string content)
    {
        await configuration.WaitAsync();
        var temporary = ConfigPath + ".editing";
        try {
            if (Locked) throw new IOException(L.T("Desbloquea rclone antes de guardar los cambios."));
            if (content.Length > 2 * 1024 * 1024 || content.Contains('\0') || content.Contains("RCLONE_ENCRYPT_V0:")) throw new IOException(L.T("Introduce una configuración INI de rclone de hasta 2 MB."));
            await File.WriteAllTextAsync(temporary, content);
            try { _ = await ExecuteAsync(CommandFor(temporary, null, "config", "dump")); }
            catch { throw new IOException(L.T("rclone no pudo leer esta configuración. Revisa las secciones [conexión] y las opciones clave = valor.")); }
            var encrypted = Encrypted;
            if (encrypted) await EncryptFileAsync(temporary, password!);
            lock (sync) {
                RequireIdleConfiguration();
                BackupConfig();
                File.Move(temporary, ConfigPath, true);
            }
        } finally { File.Delete(temporary); File.Delete(temporary + ".old"); configuration.Release(); }
    }
    public async Task ImportConfigAsync(string path)
    {
        if (!File.Exists(path) || new FileInfo(path).Length > 2 * 1024 * 1024) throw new IOException(L.T("Selecciona un rclone.conf de hasta 2 MB."));
        if (Path.GetFullPath(path).Equals(Path.GetFullPath(ConfigPath), StringComparison.OrdinalIgnoreCase)) return;
        var content = await File.ReadAllTextAsync(path);
        if (!content.StartsWith("RCLONE_ENCRYPT_V0:", StringComparison.Ordinal) && !content.Contains("\nRCLONE_ENCRYPT_V0:")) { await SaveConfigAsync(content); return; }
        await configuration.WaitAsync();
        var temporary = ConfigPath + ".importing";
        try {
            File.Copy(path, temporary, true);
            lock (sync) { RequireIdleConfiguration(); BackupConfig(); File.Move(temporary, ConfigPath, true); password = null; }
        } finally { File.Delete(temporary); configuration.Release(); }
    }
    private void RequireIdleConfiguration()
    {
        if (tasks.Values.Any(task => !task.Process.HasExited)) throw new IOException(L.T("Detén las transferencias y montajes antes de cambiar rclone.conf."));
    }
    private void BackupConfig()
    {
        if (File.Exists(ConfigPath)) File.Copy(ConfigPath, ConfigPath + ".previous", true);
    }
    public async Task UnlockAsync(string value)
    {
        await configuration.WaitAsync();
        try {
            if (!Encrypted) throw new IOException(L.T("La configuración todavía no está cifrada."));
            if (string.IsNullOrWhiteSpace(value)) throw new ArgumentException(L.T("Introduce la contraseña de rclone."));
            await ExecuteAsync(CommandFor(ConfigPath, value, "config", "encryption", "check"));
            password = value;
        } finally { configuration.Release(); }
    }
    public async Task ProtectAsync(string value)
    {
        await configuration.WaitAsync();
        var temp = ConfigPath + ".encrypted";
        try {
            if (!File.Exists(ConfigPath)) throw new IOException(L.T("Guarda primero la conexión."));
            if (Encrypted) throw new IOException(L.T("La configuración ya está cifrada."));
            if (value.Trim().Length < 12 || value.Contains('\n') || value.Contains('\r') || value.Contains('\0')) throw new ArgumentException(L.T("Usa una contraseña de al menos 12 caracteres, sin saltos de línea."));
            lock (sync) { if (tasks.Values.Any(task => !task.Process.HasExited)) throw new IOException(L.T("Detén las operaciones antes de cifrar la configuración.")); }
            File.Copy(ConfigPath, temp, true);
            await EncryptFileAsync(temp, value);
            File.Move(temp, ConfigPath, true); password = value;
        } finally { File.Delete(temp); File.Delete(temp + ".old"); configuration.Release(); }
    }
    private async Task EncryptFileAsync(string path, string value)
    {
        var info = CommandFor(path, null, "config", "encryption", "set");
        await ExecuteAsync(info, value + "\n" + value + "\n");
        if (!File.ReadLines(path).Take(5).Any(line => line == "RCLONE_ENCRYPT_V0:")) throw new IOException(L.T("rclone no pudo cifrar la configuración."));
        await ExecuteAsync(CommandFor(path, value, "config", "encryption", "check"));
        File.Delete(path + ".old");
    }
    public async Task LockAsync() { await StopAllAsync(); password = null; }
    internal (ProcessStartInfo Command, Uri Url, string Secret) MediaCommand(string path)
    {
        if (Locked || !File.Exists(ConfigPath)) throw new IOException(L.T("Guarda y desbloquea la conexión de rclone."));
        _ = Remote(path);
        var slash = path.LastIndexOf('/');
        var folder = slash < 0 ? "/" : path[..slash];
        var name = slash < 0 ? path : path[(slash + 1)..];
        var allowed = new[] {".mp3", ".mp4", ".flac", ".m4a", ".mkv", ".avi", ".wmv", ".flv", ".ogg", ".opus", ".wav", ".webm", ".mov", ".aac", ".aiff", ".ape", ".wma", ".ogv", ".m4v", ".m2ts", ".ts", ".3gp", ".mpeg", ".mpg", ".vob", ".mp2", ".alac"};
        if (!allowed.Contains(Path.GetExtension(name).ToLowerInvariant())) throw new ArgumentException(L.T("Selecciona un archivo de audio o vídeo compatible con el reproductor."));
        var listener = new TcpListener(IPAddress.Loopback, 0); listener.Start(); var port = ((IPEndPoint)listener.LocalEndpoint).Port; listener.Stop();
        var secret = Convert.ToHexString(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var command = Command("serve", "http", Remote(folder), "--addr", $"127.0.0.1:{port}", "--read-only", "--vfs-cache-mode", "off", "--buffer-size", "16Mi");
        command.Environment["RCLONE_USER"] = "teldrive-player"; command.Environment["RCLONE_PASS"] = secret;
        return (command, new Uri($"http://127.0.0.1:{port}/{Uri.EscapeDataString(name)}"), secret);
    }
    internal async Task<JsonElement> MediaInfoAsync(string path) => JsonSerializer.Deserialize<JsonElement>(await QueryAsync("lsjson", Remote(path), "--stat"));
    public async Task<JsonElement> BrowseAsync(string path) => JsonSerializer.Deserialize<JsonElement>(await QueryAsync("lsjson", Remote(path), "--no-mimetype"));
    public async Task<JsonElement> SizeAsync(string path) => JsonSerializer.Deserialize<JsonElement>(await QueryAsync("size", Remote(path), "--json"));
    public async Task<string> LinkAsync(string path) => (await QueryAsync("link", Remote(path))).Trim();
    public async Task CreateFolderAsync(string path) => _ = await QueryAsync("mkdir", Remote(path));
    public async Task RenameAsync(string source, string target)
    {
        if (source.Trim('/') == "") throw new ArgumentException(L.T("No se puede renombrar la raíz."));
        _ = await QueryAsync("moveto", Remote(source), Remote(target), "--immutable");
    }
    public async Task TrashAsync(string path, bool directory)
    {
        if (path.Trim('/') == "") throw new ArgumentException(L.T("No se puede eliminar la raíz."));
        _ = await QueryAsync(directory ? "purge" : "deletefile", Remote(path));
    }
    private async Task<string> QueryAsync(params string[] args)
    {
        if (!File.Exists(ConfigPath)) throw new IOException(L.T("Guarda primero la conexión de rclone."));
        if (Locked) throw new IOException(L.T("Desbloquea la configuración de rclone."));
        return await ExecuteAsync(Command(args));
    }
    private async Task<string> ExecuteAsync(ProcessStartInfo info, string? input = null)
    {
        using var process = Process.Start(info) ?? throw new IOException(L.T("No se pudo iniciar rclone."));
        owner.Attach(process);
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(90));
        var output = process.StandardOutput.ReadToEndAsync(deadline.Token); var errors = process.StandardError.ReadToEndAsync(deadline.Token);
        if (input is not null) await process.StandardInput.WriteAsync(input.AsMemory(), deadline.Token);
        process.StandardInput.Close();
        try { await process.WaitForExitAsync(deadline.Token); } catch { if (!process.HasExited) process.Kill(true); throw; }
        var result = await output;
        if (result.Length > 32 * 1024 * 1024) throw new IOException(L.T("La carpeta contiene demasiados elementos. Abre una subcarpeta."));
        if (process.ExitCode != 0) throw new IOException((await errors).Trim());
        return result;
    }
    private ProcessStartInfo Command(params string[] args) => CommandFor(ConfigPath, password, args);
    private ProcessStartInfo CommandFor(string configPath, string? secret, params string[] args)
    {
        var info = new ProcessStartInfo(Path.Combine(runtime.Components, "rclone.exe")) { UseShellExecute = false, CreateNoWindow = true, RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true, WorkingDirectory = runtime.DataRoot };
        foreach (var key in info.Environment.Keys.Where(key => key.StartsWith("RCLONE_", StringComparison.OrdinalIgnoreCase)).ToArray()) info.Environment.Remove(key);
        if (secret is not null) info.Environment["RCLONE_CONFIG_PASS"] = secret;
        info.ArgumentList.Add("--config"); info.ArgumentList.Add(configPath);
        info.ArgumentList.Add("--ask-password=false");
        foreach (var arg in args) info.ArgumentList.Add(arg);
        return info;
    }
    public object[] List()
    {
        lock (sync) return tasks.Select(pair => new { id = pair.Key, label = pair.Value.Label, mount = pair.Value.Mount, status = pair.Value.Process.HasExited ? pair.Value.Cancelled ? "cancelled" : pair.Value.Process.ExitCode == 0 ? "completed" : "failed" : "running", exitCode = pair.Value.Process.HasExited ? (int?)pair.Value.Process.ExitCode : null, log = pair.Value.Log }).ToArray();
    }
    public string Mount(string drive)
    {
        return StartDefinition(new() { Name = L.T("Unidad {0}", drive), Operation = "mount", Source = "/", Destination = drive }, false);
    }
    public string Upload(string localFile, string remoteFolder)
    {
        if (!File.Exists(localFile)) throw new IOException(L.T("El archivo seleccionado ya no existe."));
        var destination = Remote(remoteFolder).TrimEnd('/') + "/" + Path.GetFileName(localFile);
        return Start(L.T("Subir {0}", Path.GetFileName(localFile)), false, "copyto", localFile, destination, "--immutable", "--stats", "2s");
    }
    public string UploadFolder(string localFolder, string remoteFolder)
    {
        if (!Directory.Exists(localFolder)) throw new IOException(L.T("La carpeta seleccionada ya no existe."));
        return Start(L.T("Subir carpeta {0}", Path.GetFileName(localFolder)), false, "copy", localFolder, Remote(remoteFolder).TrimEnd('/') + "/" + Path.GetFileName(localFolder), "--immutable", "--stats", "2s");
    }
    public string Retry(string id)
    {
        RcloneTask task; lock (sync) { if (!tasks.TryGetValue(id, out task!) || !task.Process.HasExited || task.Mount) throw new IOException(L.T("Solo se pueden reintentar transferencias finalizadas.")); }
        return Start(task.Label, false, task.Arguments);
    }
    public string Download(string remotePath, string localFolder)
    {
        if (!Directory.Exists(localFolder)) throw new IOException(L.T("La carpeta seleccionada ya no existe."));
        return Start(L.T("Descargar {0}", remotePath), false, "copy", Remote(remotePath), localFolder, "--immutable", "--stats", "2s");
    }
    internal static string Remote(string path, string remoteName = "teldrive")
    {
        if (path.Contains(':') || path.Contains('\0') || path.Contains('\\') || path.Split('/').Any(part => part is ".." or ".")) throw new ArgumentException(L.T("Introduce una ruta de Teldrive sin '..' ni nombres de conexiones."));
        if (!Regex.IsMatch(remoteName, @"^[A-Za-z][\w .-]{0,63}$")) throw new ArgumentException(L.T("Selecciona una conexión válida de rclone.conf."));
        return remoteName + ":/" + path.Trim('/');
    }
    private string Start(string label, bool mount, params string[] args)
    {
        if (!File.Exists(ConfigPath)) throw new IOException(L.T("Guarda primero la conexión de rclone."));
        if (Locked) throw new IOException(L.T("Desbloquea la configuración de rclone."));
        lock (sync) {
            if (!mount && tasks.Values.Count(task => !task.Mount && !task.Process.HasExited) >= 3) throw new IOException(L.T("Ya hay tres transferencias en curso. Espera o detén una."));
            foreach (var id in tasks.Where(pair => pair.Value.Process.HasExited).Select(pair => pair.Key).Take(Math.Max(0, tasks.Count - 20)).ToArray()) { tasks[id].Process.Dispose(); tasks.Remove(id); }
            var idNew = Guid.NewGuid().ToString("N");
            var info = Command(args);
            var process = new Process { StartInfo = info, EnableRaisingEvents = true };
            var task = new RcloneTask(process, label, mount, args);
            process.OutputDataReceived += (_, item) => Append(task, item.Data);
            process.ErrorDataReceived += (_, item) => Append(task, item.Data);
            if (!process.Start()) throw new IOException(L.T("No se pudo iniciar rclone."));
            owner.Attach(process); process.StandardInput.Close(); process.BeginOutputReadLine(); process.BeginErrorReadLine(); tasks.Add(idNew, task);
            return idNew;
        }
    }
    private void Append(RcloneTask task, string? line) { if (line is null) return; lock (sync) { task.Log += line + "\n"; if (task.Log.Length > 12000) task.Log = task.Log[^12000..]; } }
    public async Task StopAsync(string id)
    {
        RcloneTask task; lock (sync) { if (!tasks.TryGetValue(id, out task!)) throw new ArgumentException(L.T("La operación ya no existe.")); if (task.Process.HasExited) return; task.Cancelled = true; }
        if (!task.Process.HasExited) { task.Process.Kill(true); await task.Process.WaitForExitAsync(); }
    }
    public async Task StopAllAsync() { string[] ids; lock (sync) ids = tasks.Keys.ToArray(); foreach (var id in ids) await StopAsync(id); }
    public async ValueTask DisposeAsync() { try { await StopAllAsync(); lock (sync) { foreach (var task in tasks.Values) task.Process.Dispose(); tasks.Clear(); } } finally { owner.Dispose(); } }
    private sealed class RcloneTask(Process process, string label, bool mount, string[] arguments) { public Process Process { get; } = process; public string Label { get; } = label; public bool Mount { get; } = mount; public string[] Arguments { get; } = arguments; public string Log { get; set; } = ""; public bool Cancelled { get; set; } }
}
