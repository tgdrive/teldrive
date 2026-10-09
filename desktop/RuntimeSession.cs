using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Net;
using System.Net.Http;
using System.Net.Sockets;
using System.Reflection;
using System.Security.AccessControl;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Text.Json;

namespace Teldrive.Desktop;

public sealed record DesktopState(int HttpPort, bool LocalDatabase, string DatabaseUrl, int DatabasePort, string DatabasePassword, string AllowedUser = "", string HttpHost = "127.0.0.1");

public sealed class RuntimeSession : IAsyncDisposable
{
    public string DataRoot { get; }
    public string Components { get; private set; } = "";
    public string ConfigPath => Path.Combine(DataRoot, "config.toml");
    public DesktopState? State { get; private set; }
    public Uri? ServerUri => State is null ? null : new UriBuilder("http", State.HttpHost, State.HttpPort).Uri;
    public bool Running => server is { HasExited: false };
    public event Action<string>? Activity;
    private Process? server;
    private Process? postgres;
    private readonly SemaphoreSlim operations = new(1, 1);
    private readonly List<Process> rcloneJobs = [];
    private readonly object logLock = new();
    private readonly WindowsJob processJob = new();

    public RuntimeSession(string? root = null)
    {
        DataRoot = root ?? Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), "TeldriveV2");
        Directory.CreateDirectory(DataRoot);
        var identity = WindowsIdentity.GetCurrent().User ?? throw new IOException("No se pudo identificar al usuario de Windows.");
        var security = new DirectorySecurity();
        security.SetAccessRuleProtection(true, false);
        security.AddAccessRule(new FileSystemAccessRule(identity, FileSystemRights.FullControl, InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit, PropagationFlags.None, AccessControlType.Allow));
        security.AddAccessRule(new FileSystemAccessRule(new SecurityIdentifier(WellKnownSidType.LocalSystemSid, null), FileSystemRights.FullControl, InheritanceFlags.ContainerInherit | InheritanceFlags.ObjectInherit, PropagationFlags.None, AccessControlType.Allow));
        new DirectoryInfo(DataRoot).SetAccessControl(security);
        if (File.Exists(Path.Combine(DataRoot, "desktop.json"))) State = JsonSerializer.Deserialize<DesktopState>(File.ReadAllText(Path.Combine(DataRoot, "desktop.json")));
    }

    public async Task PrepareAsync()
    {
        await Task.Run(() => {
            using var payload = Assembly.GetExecutingAssembly().GetManifestResourceStream("Teldrive.Desktop.Payload.zip") ?? throw new IOException("Este ejecutable no contiene el paquete de componentes. Recompila con Payload.zip.");
            var digest = Convert.ToHexString(SHA256.HashData(payload)).ToLowerInvariant();
            payload.Position = 0;
            Components = Path.Combine(DataRoot, "components", digest[..16]);
            if (!File.Exists(Path.Combine(Components, ".complete"))) {
                Directory.CreateDirectory(Components);
                using var archive = new ZipArchive(payload, ZipArchiveMode.Read);
                foreach (var entry in archive.Entries) {
                    var target = Path.GetFullPath(Path.Combine(Components, entry.FullName));
                    if (!target.StartsWith(Path.GetFullPath(Components) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new IOException("Ruta de componente inválida.");
                    if (entry.FullName.EndsWith('/')) { Directory.CreateDirectory(target); continue; }
                    Directory.CreateDirectory(Path.GetDirectoryName(target)!);
                    entry.ExtractToFile(target, true);
                }
                File.WriteAllText(Path.Combine(Components, ".complete"), digest);
            }
            var pgRoot = Path.Combine(Components, "pgsql");
            if (!File.Exists(Path.Combine(pgRoot, "share", "postgres.bki"))) ZipFile.ExtractToDirectory(Path.Combine(Components, "postgres.zip"), pgRoot, true);
        });
        Log("Componentes preparados: Teldrive v2, rclone, PostgreSQL, FFmpeg y WinFsp.");
    }

    public async Task ConfigureAsync(int port, bool local, string databaseUrl, string username)
    {
        if (port is < 1024 or > 65535) throw new ArgumentException("El puerto debe estar entre 1024 y 65535.");
        if (!local && (!Uri.TryCreate(databaseUrl, UriKind.Absolute, out var url) || url.Scheme is not ("postgres" or "postgresql"))) throw new ArgumentException("Introduce una URL de PostgreSQL válida.");
        username = username.Trim().TrimStart('@');
        if (username.Length > 0 && !System.Text.RegularExpressions.Regex.IsMatch(username, @"^[A-Za-z0-9_]{5,32}$")) throw new ArgumentException("Revisa el nombre de usuario de Telegram.");
        await StopAsync();
        var dbPort = State?.DatabasePort ?? FreePort();
        var dbPassword = State?.DatabasePassword ?? Convert.ToHexString(RandomNumberGenerator.GetBytes(32)).ToLowerInvariant();
        State = new DesktopState(port, local, databaseUrl, dbPort, dbPassword, username);
        File.WriteAllText(Path.Combine(DataRoot, "desktop.json"), JsonSerializer.Serialize(State));
        var dsn = local ? LocalDsn : databaseUrl;
        if (!File.Exists(ConfigPath)) {
            var signing = Convert.ToHexString(RandomNumberGenerator.GetBytes(32)).ToLowerInvariant();
            var dataKey = Convert.ToBase64String(RandomNumberGenerator.GetBytes(32));
            var users = username.Length > 0 ? JsonSerializer.Serialize(new[] { username }) : "[]";
            var config = $"[http]\naddress = \"127.0.0.1:{port}\"\n\n[database]\nurl = {JsonSerializer.Serialize(dsn)}\nschema = \"teldrive\"\n\n[security]\nsigning-key = \"{signing}\"\ndata-key = \"{dataKey}\"\nallowed-users = {users}\n\n[telegram]\nbackend = \"remote\"\n\n[jobs]\nrun-workers = true\n\n[logging]\nlog-level = \"info\"\nlog-format = \"text\"\n";
            await SaveConfigAsync(config);
        } else {
            var config = File.ReadAllText(ConfigPath);
            config = SetTomlValue(config, "http", "address", JsonSerializer.Serialize($"127.0.0.1:{port}"));
            config = SetTomlValue(config, "database", "url", JsonSerializer.Serialize(dsn));
            config = SetTomlValue(config, "security", "allowed-users", JsonSerializer.Serialize(username.Length > 0 ? new[] { username } : Array.Empty<string>()));
            await SaveConfigAsync(config);
        }
        await StartAsync();
    }

    private string LocalDsn => $"postgres://teldrive:{Uri.EscapeDataString(State!.DatabasePassword)}@127.0.0.1:{State.DatabasePort}/teldrive?sslmode=disable";
    private static int FreePort() { var listener = new TcpListener(IPAddress.Loopback, 0); listener.Start(); var port = ((IPEndPoint)listener.LocalEndpoint).Port; listener.Stop(); return port; }
    private string Binary(string name) => Path.Combine(Components, name);
    private static string SetTomlValue(string config, string section, string key, string value)
    {
        var lines = config.Replace("\r\n", "\n").Split('\n').ToList();
        var start = lines.FindIndex(line => line.Trim() == $"[{section}]");
        if (start < 0) { lines.Add($"[{section}]"); lines.Add($"{key} = {value}"); return string.Join('\n', lines); }
        var end = lines.FindIndex(start + 1, line => line.TrimStart().StartsWith('['));
        if (end < 0) end = lines.Count;
        var existing = lines.FindIndex(start + 1, end - start - 1, line => System.Text.RegularExpressions.Regex.IsMatch(line, @"^\s*" + System.Text.RegularExpressions.Regex.Escape(key) + @"\s*="));
        if (existing >= 0) lines[existing] = $"{key} = {value}"; else lines.Insert(start + 1, $"{key} = {value}");
        return string.Join('\n', lines);
    }

    private ProcessStartInfo Command(string binary, params string[] arguments)
    {
        var info = new ProcessStartInfo(binary) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = DataRoot, RedirectStandardOutput = true, RedirectStandardError = true };
        foreach (var argument in arguments) info.ArgumentList.Add(argument);
        info.Environment["PATH"] = Components + ";" + Path.Combine(Components, "pgsql", "bin") + ";" + Environment.GetEnvironmentVariable("PATH");
        foreach (var key in info.Environment.Keys.Where(key => key.StartsWith("TELDRIVE_", StringComparison.OrdinalIgnoreCase)).ToArray()) info.Environment.Remove(key);
        if (State is not null) info.Environment["PGPASSWORD"] = State.DatabasePassword;
        return info;
    }

    private async Task<string> RunAsync(string binary, params string[] arguments)
    {
        using var process = Process.Start(Command(binary, arguments)) ?? throw new IOException("No se pudo iniciar el componente.");
        var output = process.StandardOutput.ReadToEndAsync(); var error = process.StandardError.ReadToEndAsync();
        using var deadline = new CancellationTokenSource(TimeSpan.FromMinutes(2));
        try { await process.WaitForExitAsync(deadline.Token); } catch { if (!process.HasExited) process.Kill(true); throw; }
        var result = await output; var failure = await error;
        if (process.ExitCode != 0) throw new IOException($"{Path.GetFileName(binary)}: {failure}\n{result}");
        return result;
    }

    private Process StartService(string binary, params string[] arguments)
    {
        var process = new Process { StartInfo = Command(binary, arguments), EnableRaisingEvents = true };
        process.OutputDataReceived += (_, line) => { if (line.Data is not null) Log(line.Data); };
        process.ErrorDataReceived += (_, line) => { if (line.Data is not null) Log(line.Data); };
        if (!process.Start()) throw new IOException("No se pudo iniciar el servicio.");
        processJob.Attach(process);
        process.BeginOutputReadLine(); process.BeginErrorReadLine(); return process;
    }

    public async Task StartAsync()
    {
        await operations.WaitAsync();
        try {
            if (Running) return;
            if (State is null || !File.Exists(ConfigPath)) throw new IOException("Completa la configuración inicial.");
            await ReadHttpPortAsync(ConfigPath);
            if (State.LocalDatabase) await StartDatabaseAsync();
            await ValidateConfigAsync(File.ReadAllText(ConfigPath));
            server = StartService(Binary("teldrive.exe"), "run", "--config", ConfigPath);
            using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(2) };
            for (var attempt = 0; attempt < 120; attempt++) {
                if (server.HasExited) throw new IOException("Teldrive no pudo iniciar. Consulta Actividad para ver el motivo.");
                try { using var response = await http.GetAsync(new Uri(ServerUri!, "/health/live")); if (response.IsSuccessStatusCode) { Log("Tu unidad está lista."); return; } } catch (HttpRequestException) {} catch (TaskCanceledException) {}
                await Task.Delay(500);
            }
            throw new IOException("El servidor tardó demasiado en iniciar. Consulta Actividad.");
        } catch {
            await StopCoreAsync();
            throw;
        } finally { operations.Release(); }
    }

    private async Task StartDatabaseAsync()
    {
        var data = Path.Combine(DataRoot, "database"); var pg = Path.Combine(Components, "pgsql", "bin");
        if (!File.Exists(Path.Combine(data, "PG_VERSION"))) {
            var passwordFile = Path.Combine(DataRoot, "database-password.tmp");
            try { File.WriteAllText(passwordFile, State!.DatabasePassword); await RunAsync(Path.Combine(pg, "initdb.exe"), "-D", data, "-U", "teldrive", "-A", "scram-sha-256", "--pwfile", passwordFile, "-E", "UTF8", "--locale", "C"); } finally { File.Delete(passwordFile); }
        }
        if (postgres is not { HasExited: false }) postgres = StartService(Path.Combine(pg, "postgres.exe"), "-D", data, "-p", State!.DatabasePort.ToString(), "-h", "127.0.0.1");
        var ready = false;
        for (var attempt = 0; attempt < 60; attempt++) {
            if (postgres.HasExited) throw new IOException("PostgreSQL no pudo iniciar. Revisa los componentes de Microsoft en Configuración.");
            try { await RunAsync(Path.Combine(pg, "psql.exe"), "-h", "127.0.0.1", "-p", State!.DatabasePort.ToString(), "-U", "teldrive", "-d", "postgres", "-tAc", "SELECT 1"); ready = true; break; } catch (IOException) { await Task.Delay(500); }
        }
        if (!ready) throw new IOException("PostgreSQL no respondió.");
        var exists = await RunAsync(Path.Combine(pg, "psql.exe"), "-h", "127.0.0.1", "-p", State!.DatabasePort.ToString(), "-U", "teldrive", "-d", "postgres", "-tAc", "SELECT 1 FROM pg_database WHERE datname='teldrive'");
        if (exists.Trim() != "1") await RunAsync(Path.Combine(pg, "createdb.exe"), "-h", "127.0.0.1", "-p", State.DatabasePort.ToString(), "-U", "teldrive", "teldrive");
    }

    public async Task ValidateConfigAsync(string config)
    {
        var temporary = Path.Combine(DataRoot, $"validate-{Guid.NewGuid():N}.toml");
        try { await File.WriteAllTextAsync(temporary, config); await RunAsync(Binary("teldrive.exe"), "check", "--config", temporary, "--validate-only"); } finally { File.Delete(temporary); }
    }
    public async Task SaveConfigAsync(string config)
    {
        await ValidateConfigAsync(config);
        var temporary = ConfigPath + ".new.toml";
        try {
            await File.WriteAllTextAsync(temporary, config);
            var endpoint = await ReadDesktopEndpointAsync(temporary);
            File.Move(temporary, ConfigPath, true);
            await SetDesktopEndpointAsync(endpoint);
        } finally { File.Delete(temporary); }
        Log("Configuración guardada y validada.");
    }
    private async Task ReadHttpPortAsync(string path)
        => await SetDesktopEndpointAsync(await ReadDesktopEndpointAsync(path));

    private sealed record DesktopEndpoint(Uri Http, bool LocalDatabase);
    private async Task<DesktopEndpoint> ReadDesktopEndpointAsync(string path)
    {
        using var data = JsonDocument.Parse(await RunAsync(Binary("teldrive.exe"), "check", "--config", path, "--desktop-info"));
        var address = data.RootElement.GetProperty("address").GetString();
        if (!Uri.TryCreate("http://" + address, UriKind.Absolute, out var endpoint) || endpoint.Port is < 1 or > 65535 || endpoint.UserInfo.Length > 0 || endpoint.AbsolutePath != "/") throw new IOException("La dirección HTTP configurada no es válida para Desktop.");
        var localDatabase = State?.LocalDatabase ?? false;
        if (data.RootElement.TryGetProperty("databaseAddress", out var databaseAddress)) {
            localDatabase = State is not null && Uri.TryCreate("http://" + databaseAddress.GetString(), UriKind.Absolute, out var database) && database.IsLoopback && database.Port == State.DatabasePort;
        }
        if (endpoint.Host is "0.0.0.0" or "[::]") return new DesktopEndpoint(new UriBuilder("http", "127.0.0.1", endpoint.Port).Uri, localDatabase);
        if (!endpoint.IsLoopback) throw new IOException("Desktop requiere una dirección local: 127.0.0.1, localhost, [::1], 0.0.0.0 o [::].");
        return new DesktopEndpoint(endpoint, localDatabase);
    }
    private async Task SetDesktopEndpointAsync(DesktopEndpoint info)
    {
        var endpoint = info.Http;
        if (State is not null && (State.HttpPort != endpoint.Port || State.HttpHost != endpoint.Host || State.LocalDatabase != info.LocalDatabase)) { State = State with {HttpPort = endpoint.Port, HttpHost = endpoint.Host, LocalDatabase = info.LocalDatabase}; await File.WriteAllTextAsync(Path.Combine(DataRoot, "desktop.json"), JsonSerializer.Serialize(State)); }
    }

    public async Task StopAsync()
    {
        await operations.WaitAsync();
        try { await StopCoreAsync(); } finally { operations.Release(); }
    }
    private async Task StopCoreAsync()
    {
            foreach (var process in rcloneJobs) { if (!process.HasExited) { process.Kill(true); await process.WaitForExitAsync(); } process.Dispose(); } rcloneJobs.Clear();
            if (server is not null) { if (!server.HasExited) { server.Kill(true); await server.WaitForExitAsync(); } server.Dispose(); server = null; }
            if (postgres is not null) { try { if (!postgres.HasExited) await RunAsync(Path.Combine(Components, "pgsql", "bin", "pg_ctl.exe"), "stop", "-D", Path.Combine(DataRoot, "database"), "-m", "fast", "-w", "-t", "20"); } finally { if (!postgres.HasExited) { postgres.Kill(true); await postgres.WaitForExitAsync(); } postgres.Dispose(); postgres = null; } }
            Log("Servicios detenidos.");
    }
    public async Task InstallAsync(string filename, bool administrator, params string[] arguments)
    {
        var info = new ProcessStartInfo(Binary(filename)) { UseShellExecute = true, Verb = administrator ? "runas" : "open" };
        foreach (var argument in arguments) info.ArgumentList.Add(argument);
        using var process = Process.Start(info) ?? throw new IOException("No se pudo abrir el instalador integrado."); await process.WaitForExitAsync();
        if (process.ExitCode is not (0 or 3010)) throw new IOException($"El instalador terminó con código {process.ExitCode}.");
        Log("Componente de Windows preparado.");
    }
    private void Log(string line) { lock (logLock) { var path = Path.Combine(DataRoot, "activity.log"); if (File.Exists(path) && new FileInfo(path).Length > 10 * 1024 * 1024) File.Move(path, path + ".previous", true); File.AppendAllText(path, $"{DateTime.Now:HH:mm:ss} {line}\n"); } Activity?.Invoke(line); }
    public async ValueTask DisposeAsync() { try { await StopAsync(); } finally { processJob.Dispose(); operations.Dispose(); } }
}
