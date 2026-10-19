using System.IO;
using System.IO.Pipes;
using System.Text;
using System.Text.Json;

namespace Teldrive.Desktop;

// Only explicit media properties are collected; URLs, HTTP headers and process arguments are excluded.
internal sealed class MediaTelemetry : IAsyncDisposable
{
    public string PipeName { get; } = "teldrive-media-" + Guid.NewGuid().ToString("N");
    private readonly NamedPipeClientStream pipe;
    private readonly SemaphoreSlim writes = new(1, 1);
    private readonly Dictionary<string, JsonElement> values = [];
    private readonly CancellationTokenSource lifetime = new();
    private Task? readerTask;
    public event Action<string>? Control;
    public bool Connected { get; private set; }
    internal static readonly string[] Properties = ["time-pos", "duration", "pause", "speed", "volume", "mute", "file-format", "file-size", "video-codec", "audio-codec-name", "video-params", "audio-params", "container-fps", "estimated-vf-fps", "decoder-frame-drop-count", "frame-drop-count", "avsync", "hwdec-current", "video-bitrate", "audio-bitrate", "cache-speed", "demuxer-cache-duration", "demuxer-cache-state", "paused-for-cache", "seeking", "eof-reached", "fullscreen"];
    public MediaTelemetry() { pipe = new NamedPipeClientStream(".", PipeName, PipeDirection.InOut, PipeOptions.Asynchronous); }
    public void Start() => readerTask = ReadAsync();
    public Dictionary<string, JsonElement> Snapshot() { lock (values) return new(values); }
    private async Task ReadAsync()
    {
        try {
            using var deadline = CancellationTokenSource.CreateLinkedTokenSource(lifetime.Token);
            deadline.CancelAfter(TimeSpan.FromSeconds(20));
            await pipe.ConnectAsync(deadline.Token); Connected = true;
            for (var i = 0; i < Properties.Length; i++) await SendAsync("observe_property", i + 1, Properties[i]);
            using var reader = new StreamReader(pipe, Encoding.UTF8, false, 8192, leaveOpen:true);
            while (await reader.ReadLineAsync(lifetime.Token) is { } line) {
                using var document = JsonDocument.Parse(line);
                var root = document.RootElement;
                if (!root.TryGetProperty("event", out var type)) continue;
                if (type.GetString() == "property-change" && root.TryGetProperty("name", out var name) && root.TryGetProperty("data", out var data)) {
                    var property = name.GetString()!;
                    if (!Properties.Contains(property)) continue;
                    if (property == "fullscreen" && data.ValueKind == JsonValueKind.True) { Control?.Invoke("fullscreen"); await SendAsync("set_property", "fullscreen", false); }
                    else { lock (values) values[property] = data.Clone(); }
                } else if (type.GetString() == "client-message" && root.TryGetProperty("args", out var args) && args.GetArrayLength() > 0) {
                    var action = args[0].GetString();
                    if (action is "teldrive-fullscreen" or "teldrive-exit-fullscreen" or "teldrive-statistics") Control?.Invoke(action);
                }
            }
        } catch (Exception error) when (error is IOException or OperationCanceledException or ObjectDisposedException or JsonException) { }
        finally { Connected = false; }
    }
    public async Task SendAsync(params object[] command)
    {
        if (!Connected || lifetime.IsCancellationRequested) return;
        await writes.WaitAsync(lifetime.Token);
        try { await pipe.WriteAsync(Encoding.UTF8.GetBytes(JsonSerializer.Serialize(new { command }) + "\n"), lifetime.Token); await pipe.FlushAsync(lifetime.Token); }
        finally { writes.Release(); }
    }
    public async ValueTask DisposeAsync() { lifetime.Cancel(); pipe.Dispose(); if (readerTask is not null) await readerTask; lifetime.Dispose(); writes.Dispose(); }
}
