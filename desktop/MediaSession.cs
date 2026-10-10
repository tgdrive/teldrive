using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;

namespace Teldrive.Desktop;

internal sealed class MediaSession : IAsyncDisposable
{
    private readonly WindowsJob owner = new();
    private readonly RuntimeSession runtime;
    private Process? server;
    private Process? player;
    private readonly string folder;
    private readonly CancellationTokenSource lifetime = new();
    private bool disposed;
    public string? Failure { get; private set; }
    public MediaSession(RuntimeSession runtime) { this.runtime = runtime; folder = Path.Combine(runtime.DataRoot, "players", Guid.NewGuid().ToString("N")); }
    public async Task StartAsync(RcloneRuntime rclone, string path, nint window, bool headless = false)
    {
        lifetime.Token.ThrowIfCancellationRequested();
        Directory.CreateDirectory(folder);
        var stream = rclone.MediaCommand(path);
        server = Process.Start(stream.Command) ?? throw new IOException("No se pudo iniciar el streaming.");
        owner.Attach(server); server.StandardInput.Close();
        var failures = server.StandardError.ReadToEndAsync(); _ = server.StandardOutput.ReadToEndAsync();
        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(3) };
        var auth = Convert.ToBase64String(Encoding.UTF8.GetBytes("teldrive-player:" + stream.Secret));
        http.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Basic", auth);
        var ready = false;
        for (var attempt = 0; attempt < 40; attempt++) {
            lifetime.Token.ThrowIfCancellationRequested();
            if (server.HasExited) throw new IOException("El streaming no pudo iniciar: " + await failures);
            try { using var request = new HttpRequestMessage(HttpMethod.Head, stream.Url); using var response = await http.SendAsync(request, lifetime.Token); if (response.IsSuccessStatusCode) { ready = true; break; } if (response.StatusCode is System.Net.HttpStatusCode.Unauthorized or System.Net.HttpStatusCode.NotFound) throw new IOException("rclone no pudo acceder al archivo multimedia."); } catch (HttpRequestException) {} catch (TaskCanceledException) { lifetime.Token.ThrowIfCancellationRequested(); }
            await Task.Delay(250, lifetime.Token);
        }
        if (!ready) throw new IOException("El streaming no respondió.");
        // Keep the short-lived credential out of process arguments and logs.
        await File.WriteAllTextAsync(Path.Combine(folder, "mpv.conf"), "http-header-fields=\"Authorization: Basic " + auth + "\"\n", lifetime.Token);
        var command = new ProcessStartInfo(Path.Combine(runtime.Components, "mpv", "mpv.exe")) { UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = folder };
        foreach (var arg in new[] {"--config-dir=" + folder, "--load-scripts=no", "--ytdl=no", "--access-references=no", "--load-unsafe-playlists=no", "--idle=no", "--keep-open=no", "--osc=yes", "--title=Teldrive multimedia"}) command.ArgumentList.Add(arg);
        if (headless) { command.ArgumentList.Add("--vo=null"); command.ArgumentList.Add("--ao=null"); }
        else command.ArgumentList.Add("--wid=" + window.ToString());
        command.ArgumentList.Add(stream.Url.AbsoluteUri);
        lifetime.Token.ThrowIfCancellationRequested();
        player = Process.Start(command) ?? throw new IOException("No se pudo iniciar el reproductor integrado."); owner.Attach(player);
        await player.WaitForExitAsync();
        if (player.ExitCode != 0) { Failure = "mpv no pudo reproducir este archivo. Comprueba su integridad y conexión."; throw new IOException(Failure); }
    }
    public async ValueTask DisposeAsync()
    {
        if (disposed) return; disposed = true; lifetime.Cancel();
        try {
            foreach (var process in new[] {player, server}) { if (process is null) continue; if (!process.HasExited) { process.Kill(true); await process.WaitForExitAsync(); } process.Dispose(); }
        } finally { owner.Dispose(); if (Directory.Exists(folder)) Directory.Delete(folder, true); }
    }
}
