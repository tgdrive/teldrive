using System.Diagnostics;
using System.IO;
using System.Text.Json;
using System.Text.RegularExpressions;
using System.Net;
using System.Net.Sockets;

namespace Teldrive.Desktop;

public sealed record RcloneConnection(string ApiKey, int ChunkMiB = 512, int Concurrency = 4, bool Encrypt = false, bool Hash = true, string LinkPassword = "");
public sealed class RcloneRuntime : IAsyncDisposable
{
    private readonly RuntimeSession runtime;
    private readonly WindowsJob owner = new();
    private readonly Dictionary<string, RcloneTask> tasks = [];
    private readonly object sync = new();
    private readonly SemaphoreSlim configuration = new(1, 1);
    private string? password;
    public bool Encrypted => File.Exists(ConfigPath) && File.ReadLines(ConfigPath).Take(5).Any(line => line == "RCLONE_ENCRYPT_V0:");
    public bool Locked => Encrypted && password is null;
    public RcloneRuntime(RuntimeSession runtime) { this.runtime = runtime; }
    private string ConfigPath => Path.Combine(runtime.DataRoot, "rclone.conf");
    public bool DriverInstalled => File.Exists(Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86), "WinFsp", "bin", "winfsp-x64.dll"));

    public async Task ConfigureAsync(RcloneConnection connection)
    {
        await configuration.WaitAsync();
        try {
        if (Locked) throw new IOException("Desbloquea la configuración antes de modificar la conexión.");
        if (connection.ApiKey.Length < 8 || (connection.ApiKey + connection.LinkPassword).IndexOfAny(['\r', '\n', '\0']) >= 0) throw new ArgumentException("Introduce una clave de API válida.");
        if (connection.ChunkMiB is < 64 or > 2000 || connection.ChunkMiB % 16 != 0 || connection.Concurrency is < 1 or > 32) throw new ArgumentException("Revisa el tamaño de fragmento y la concurrencia.");
        lock (sync) { if (tasks.Values.Any(task => !task.Process.HasExited)) throw new IOException("Detén las operaciones de rclone antes de cambiar su conexión."); }
        var config = $"[teldrive]\ntype = teldrive\napi_host = {runtime.ServerUri!.GetLeftPart(UriPartial.Authority)}\napi_key = {connection.ApiKey}\nchunk_size = {connection.ChunkMiB}Mi\nupload_concurrency = {connection.Concurrency}\nencrypt_files = {connection.Encrypt.ToString().ToLowerInvariant()}\nhash_enabled = {connection.Hash.ToString().ToLowerInvariant()}\npage_size = 200\n";
        if (!string.IsNullOrEmpty(connection.LinkPassword)) config += $"link_password = {connection.LinkPassword}\n";
        var temp = ConfigPath + ".new";
        try {
            await File.WriteAllTextAsync(temp, config);
            if (Encrypted) await EncryptFileAsync(temp, password!);
            File.Move(temp, ConfigPath, true);
        } finally { File.Delete(temp); File.Delete(temp + ".old"); }
        } finally { configuration.Release(); }
    }

    public object Info() => new { configured = File.Exists(ConfigPath), encrypted = Encrypted, locked = Locked, driverInstalled = DriverInstalled, platform = "windows", version = "1.76.0-teldrive-v2.1" };
    public async Task UnlockAsync(string value)
    {
        await configuration.WaitAsync();
        try {
            if (!Encrypted) throw new IOException("La configuración todavía no está cifrada.");
            if (string.IsNullOrWhiteSpace(value)) throw new ArgumentException("Introduce la contraseña de rclone.");
            await ExecuteAsync(CommandFor(ConfigPath, value, "config", "encryption", "check"));
            password = value;
        } finally { configuration.Release(); }
    }
    public async Task ProtectAsync(string value)
    {
        await configuration.WaitAsync();
        var temp = ConfigPath + ".encrypted";
        try {
            if (!File.Exists(ConfigPath)) throw new IOException("Guarda primero la conexión.");
            if (Encrypted) throw new IOException("La configuración ya está cifrada.");
            if (value.Trim().Length < 12 || value.Contains('\n') || value.Contains('\r') || value.Contains('\0')) throw new ArgumentException("Usa una contraseña de al menos 12 caracteres, sin saltos de línea.");
            lock (sync) { if (tasks.Values.Any(task => !task.Process.HasExited)) throw new IOException("Detén las operaciones antes de cifrar la configuración."); }
            File.Copy(ConfigPath, temp, true);
            await EncryptFileAsync(temp, value);
            File.Move(temp, ConfigPath, true); password = value;
        } finally { File.Delete(temp); File.Delete(temp + ".old"); configuration.Release(); }
    }
    private async Task EncryptFileAsync(string path, string value)
    {
        var info = CommandFor(path, null, "config", "encryption", "set");
        await ExecuteAsync(info, value + "\n" + value + "\n");
        if (!File.ReadLines(path).Take(5).Any(line => line == "RCLONE_ENCRYPT_V0:")) throw new IOException("rclone no pudo cifrar la configuración.");
        await ExecuteAsync(CommandFor(path, value, "config", "encryption", "check"));
        File.Delete(path + ".old");
    }
    public async Task LockAsync() { await StopAllAsync(); password = null; }
    internal (ProcessStartInfo Command, Uri Url, string Secret) MediaCommand(string path)
    {
        if (Locked || !File.Exists(ConfigPath)) throw new IOException("Guarda y desbloquea la conexión de rclone.");
        _ = Remote(path);
        var slash = path.LastIndexOf('/');
        var folder = slash < 0 ? "/" : path[..slash];
        var name = slash < 0 ? path : path[(slash + 1)..];
        var allowed = new[] {".mp3", ".mp4", ".flac", ".m4a", ".mkv", ".avi", ".wmv", ".flv", ".ogg", ".opus", ".wav", ".webm", ".mov", ".aac", ".aiff", ".ape", ".wma", ".ogv", ".m4v", ".m2ts", ".ts", ".3gp", ".mpeg", ".mpg", ".vob", ".mp2", ".alac"};
        if (!allowed.Contains(Path.GetExtension(name).ToLowerInvariant())) throw new ArgumentException("Selecciona un archivo de audio o vídeo compatible con el reproductor.");
        var listener = new TcpListener(IPAddress.Loopback, 0); listener.Start(); var port = ((IPEndPoint)listener.LocalEndpoint).Port; listener.Stop();
        var secret = Convert.ToHexString(System.Security.Cryptography.RandomNumberGenerator.GetBytes(32));
        var command = Command("serve", "http", Remote(folder), "--addr", $"127.0.0.1:{port}", "--read-only", "--vfs-cache-mode", "off", "--buffer-size", "16Mi");
        command.Environment["RCLONE_USER"] = "teldrive-player"; command.Environment["RCLONE_PASS"] = secret;
        return (command, new Uri($"http://127.0.0.1:{port}/{Uri.EscapeDataString(name)}"), secret);
    }
    public async Task<JsonElement> BrowseAsync(string path) => JsonSerializer.Deserialize<JsonElement>(await QueryAsync("lsjson", Remote(path), "--no-mimetype"));
    public async Task<JsonElement> SizeAsync(string path) => JsonSerializer.Deserialize<JsonElement>(await QueryAsync("size", Remote(path), "--json"));
    public async Task<string> LinkAsync(string path) => (await QueryAsync("link", Remote(path))).Trim();
    public async Task CreateFolderAsync(string path) => _ = await QueryAsync("mkdir", Remote(path));
    public async Task RenameAsync(string source, string target)
    {
        if (source.Trim('/') == "") throw new ArgumentException("No se puede renombrar la raíz.");
        _ = await QueryAsync("moveto", Remote(source), Remote(target), "--immutable");
    }
    public async Task TrashAsync(string path, bool directory)
    {
        if (path.Trim('/') == "") throw new ArgumentException("No se puede eliminar la raíz.");
        _ = await QueryAsync(directory ? "purge" : "deletefile", Remote(path));
    }
    private async Task<string> QueryAsync(params string[] args)
    {
        if (!File.Exists(ConfigPath)) throw new IOException("Guarda primero la conexión de rclone.");
        if (Locked) throw new IOException("Desbloquea la configuración de rclone.");
        return await ExecuteAsync(Command(args));
    }
    private async Task<string> ExecuteAsync(ProcessStartInfo info, string? input = null)
    {
        using var process = Process.Start(info) ?? throw new IOException("No se pudo iniciar rclone.");
        owner.Attach(process);
        using var deadline = new CancellationTokenSource(TimeSpan.FromSeconds(90));
        var output = process.StandardOutput.ReadToEndAsync(deadline.Token); var errors = process.StandardError.ReadToEndAsync(deadline.Token);
        if (input is not null) await process.StandardInput.WriteAsync(input.AsMemory(), deadline.Token);
        process.StandardInput.Close();
        try { await process.WaitForExitAsync(deadline.Token); } catch { if (!process.HasExited) process.Kill(true); throw; }
        var result = await output;
        if (result.Length > 32 * 1024 * 1024) throw new IOException("La carpeta contiene demasiados elementos. Abre una subcarpeta.");
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
        if (!DriverInstalled) throw new IOException("Prepara WinFsp desde este panel antes de montar la unidad.");
        drive = drive.Trim().ToUpperInvariant();
        if (!Regex.IsMatch(drive, "^[D-Z]:$")) throw new ArgumentException("Elige una letra libre entre D: y Z:.");
        if (DriveInfo.GetDrives().Any(item => item.Name.StartsWith(drive, StringComparison.OrdinalIgnoreCase))) throw new IOException("Esa letra de unidad ya está en uso.");
        lock (sync) { if (tasks.Values.Any(task => task.Mount && !task.Process.HasExited)) throw new IOException("Ya hay una unidad montada. Detén el montaje anterior."); }
        return Start($"Unidad {drive}", true, "mount", "teldrive:/", drive, "--vfs-cache-mode", "full", "--cache-dir", Path.Combine(runtime.DataRoot, "cache", "rclone"), "--vfs-cache-max-size", "10Gi", "--vfs-cache-max-age", "24h", "--volname", "Teldrive v2");
    }
    public string Upload(string localFile, string remoteFolder)
    {
        if (!File.Exists(localFile)) throw new IOException("El archivo seleccionado ya no existe.");
        var destination = Remote(remoteFolder).TrimEnd('/') + "/" + Path.GetFileName(localFile);
        return Start($"Subir {Path.GetFileName(localFile)}", false, "copyto", localFile, destination, "--immutable", "--stats", "2s");
    }
    public string UploadFolder(string localFolder, string remoteFolder)
    {
        if (!Directory.Exists(localFolder)) throw new IOException("La carpeta seleccionada ya no existe.");
        return Start($"Subir carpeta {Path.GetFileName(localFolder)}", false, "copy", localFolder, Remote(remoteFolder).TrimEnd('/') + "/" + Path.GetFileName(localFolder), "--immutable", "--stats", "2s");
    }
    public string Retry(string id)
    {
        RcloneTask task; lock (sync) { if (!tasks.TryGetValue(id, out task!) || !task.Process.HasExited || task.Mount) throw new IOException("Solo se pueden reintentar transferencias finalizadas."); }
        return Start(task.Label, false, task.Arguments);
    }
    public string Download(string remotePath, string localFolder)
    {
        if (!Directory.Exists(localFolder)) throw new IOException("La carpeta seleccionada ya no existe.");
        return Start($"Descargar {remotePath}", false, "copy", Remote(remotePath), localFolder, "--immutable", "--stats", "2s");
    }
    private static string Remote(string path)
    {
        if (path.Contains(':') || path.Contains('\0') || path.Contains('\\') || path.Split('/').Any(part => part is ".." or ".")) throw new ArgumentException("Introduce una ruta de Teldrive sin '..' ni nombres de conexiones.");
        return "teldrive:/" + path.Trim('/');
    }
    private string Start(string label, bool mount, params string[] args)
    {
        if (!File.Exists(ConfigPath)) throw new IOException("Guarda primero la conexión de rclone.");
        if (Locked) throw new IOException("Desbloquea la configuración de rclone.");
        lock (sync) {
            if (tasks.Values.Count(task => !task.Process.HasExited) >= 3) throw new IOException("Ya hay tres operaciones en curso. Espera o detén una.");
            foreach (var id in tasks.Where(pair => pair.Value.Process.HasExited).Select(pair => pair.Key).Take(Math.Max(0, tasks.Count - 20)).ToArray()) { tasks[id].Process.Dispose(); tasks.Remove(id); }
            var idNew = Guid.NewGuid().ToString("N");
            var info = Command(args);
            var process = new Process { StartInfo = info, EnableRaisingEvents = true };
            var task = new RcloneTask(process, label, mount, args);
            process.OutputDataReceived += (_, item) => Append(task, item.Data);
            process.ErrorDataReceived += (_, item) => Append(task, item.Data);
            if (!process.Start()) throw new IOException("No se pudo iniciar rclone.");
            owner.Attach(process); process.StandardInput.Close(); process.BeginOutputReadLine(); process.BeginErrorReadLine(); tasks.Add(idNew, task);
            return idNew;
        }
    }
    private void Append(RcloneTask task, string? line) { if (line is null) return; lock (sync) { task.Log += line + "\n"; if (task.Log.Length > 12000) task.Log = task.Log[^12000..]; } }
    public async Task StopAsync(string id)
    {
        RcloneTask task; lock (sync) { if (!tasks.TryGetValue(id, out task!)) throw new ArgumentException("La operación ya no existe."); task.Cancelled = true; }
        if (!task.Process.HasExited) { task.Process.Kill(true); await task.Process.WaitForExitAsync(); }
    }
    public async Task StopAllAsync() { string[] ids; lock (sync) ids = tasks.Keys.ToArray(); foreach (var id in ids) await StopAsync(id); }
    public async ValueTask DisposeAsync() { try { await StopAllAsync(); lock (sync) { foreach (var task in tasks.Values) task.Process.Dispose(); tasks.Clear(); } } finally { owner.Dispose(); } }
    private sealed class RcloneTask(Process process, string label, bool mount, string[] arguments) { public Process Process { get; } = process; public string Label { get; } = label; public bool Mount { get; } = mount; public string[] Arguments { get; } = arguments; public string Log { get; set; } = ""; public bool Cancelled { get; set; } }
}
