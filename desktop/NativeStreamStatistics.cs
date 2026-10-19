using System.IO;
using System.Net.Http;
using System.Text.Json;

namespace Teldrive.Desktop;

internal sealed class NativeStreamStatistics : IDisposable
{
    private readonly HttpClient http;
    private readonly Uri endpoint;
    private NativeStreamStatistics(Uri endpoint, string key) {
        this.endpoint = endpoint;
        http = new HttpClient(new HttpClientHandler { AllowAutoRedirect = false }) { Timeout = TimeSpan.FromSeconds(5) };
        http.DefaultRequestHeaders.Add("X-Api-Key", key);
    }
    public static async Task<NativeStreamStatistics?> CreateAsync(RcloneRuntime rclone, string path)
    {
        var options = RcloneConfigDocument.Options(await rclone.ReadConfigAsync(), "teldrive").ToDictionary(item => item.Key, item => item.Value);
        if (options.GetValueOrDefault("type") != "teldrive" || !Uri.TryCreate(options.GetValueOrDefault("api_host"), UriKind.Absolute, out var host) || host.Scheme is not ("http" or "https")) return null;
        var key = options.GetValueOrDefault("api_key"); if (string.IsNullOrEmpty(key)) return null;
        var file = await rclone.MediaInfoAsync(path);
        if (!file.TryGetProperty("ID", out var id) || !Guid.TryParse(id.GetString(), out var guid)) return null;
        return new NativeStreamStatistics(new Uri(host, "/api/v1/files/" + guid + "/playback/statistics"), key);
    }
    public async Task<string> ReadAsync(CancellationToken cancel)
    {
        using var response = await http.GetAsync(endpoint, cancel);
        if (!response.IsSuccessStatusCode) return "Estadísticas de Telegram no disponibles (HTTP " + (int)response.StatusCode + ").";
        using var document = JsonDocument.Parse(await response.Content.ReadAsStringAsync(cancel));
        return Format(document.RootElement);
    }
    internal static string Format(JsonElement root)
    {
        var text = new System.Text.StringBuilder("TELEGRAM · flujos de este archivo\n");
        string Value(JsonElement row, string key) => row.TryGetProperty(key, out var value) ? value.ToString() : "—";
        text.AppendLine("Actualizado: " + Value(root, "sampledAt"));
        text.AppendLine("Flujos abiertos: " + Value(root, "activeStreams"));
        text.AppendLine("Recibido: " + Value(root, "bytes") + " B · " + Value(root, "bytesPerSecond") + " B/s");
        text.AppendLine("Velocidad media móvil de 2 s; incluye búsquedas y lecturas simultáneas del archivo.");
        if (root.TryGetProperty("accounts", out var accounts)) {
            if (accounts.GetArrayLength() == 0) text.AppendLine("Sin solicitudes de Telegram registradas; puede estar leyendo del búfer.");
            foreach (var row in accounts.EnumerateArray()) {
                text.AppendLine(); text.AppendLine(Value(row,"name") + " · @" + Value(row,"alias") + " · ID " + Value(row,"id"));
                text.AppendLine(Value(row,"bytesPerSecond") + " B/s · " + Value(row,"bytes") + " B recibidos");
                text.AppendLine("Solicitudes activas: " + Value(row,"activeRequests") + " · total: " + Value(row,"requests") + " · errores: " + Value(row,"errors"));
                text.AppendLine("Última latencia: " + Value(row,"latencyMs") + " ms · última recepción: " + Value(row,"lastSeen"));
                text.AppendLine("Canal: " + Value(row,"channelId") + " · mensaje: " + Value(row,"messageId") + " · offset: " + Value(row,"offset"));
            }
        }
        return text.ToString();
    }
    public void Dispose() => http.Dispose();
}
