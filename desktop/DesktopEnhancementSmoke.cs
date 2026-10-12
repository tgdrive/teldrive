using System.IO;
using System.Security.Cryptography;
using System.Text.Json;

namespace Teldrive.Desktop;

internal static class DesktopEnhancementSmoke
{
    public static async Task RunAsync(RuntimeSession runtime, MainWindow window)
    {
        if (runtime.State is not null || File.Exists(runtime.ConfigPath)) throw new IOException("La prueba requiere una carpeta nueva.");
        var listener = new System.Net.Sockets.TcpListener(System.Net.IPAddress.Loopback, 0); listener.Start(); var port = ((System.Net.IPEndPoint)listener.LocalEndpoint).Port; listener.Stop();
        await runtime.ConfigureAsync(port, true, "", "");
        await window.VerifyNavigationAsync();
        var remote = Path.Combine(runtime.DataRoot, "remote-a"); var other = Path.Combine(runtime.DataRoot, "remote-b");
        Directory.CreateDirectory(remote); Directory.CreateDirectory(other);
        var imported = Path.Combine(runtime.DataRoot, "custom-input.conf");
        var content = $"# comentarios conservados\n[teldrive]\ntype = alias\nremote = {remote}\ncustom_option = original\n\n[otra nube]\ntype = alias\nremote = {other}\n";
        await File.WriteAllTextAsync(imported, content); var original = SHA256.HashData(await File.ReadAllBytesAsync(imported));
        await using var rclone = new RcloneRuntime(runtime);
        await rclone.ImportConfigAsync(imported);
        var importedHash = SHA256.HashData(await File.ReadAllBytesAsync(imported));
        Check(original.SequenceEqual(importedHash), "Se modificó el archivo importado.");
        var loaded = await rclone.ReadConfigAsync(); Check(loaded.Contains("# comentarios conservados") && RcloneConfigDocument.Sections(loaded).Length == 2, "Se perdió una conexión o comentario.");
        loaded = RcloneConfigDocument.Set(loaded, "teldrive", "custom_option", "editado"); await rclone.SaveConfigAsync(loaded);
        Check(File.Exists(rclone.ConfigFilePath + ".previous") && (await rclone.ReadConfigAsync()).Contains("custom_option = editado"), "No se guardó la opción o su respaldo.");
        var source = Path.Combine(runtime.DataRoot, "upload-test.txt"); await File.WriteAllTextAsync(source, "contenido íntegro");
        var job = rclone.StartDefinition(new RcloneTaskDefinition { Name = "Conexión personalizada", RemoteName = "otra nube", Source = source, Destination = "/" }, false);
        for (var attempt = 0; attempt < 80 && rclone.Snapshot(job)?.State == "running"; attempt++) await Task.Delay(100);
        Check(File.Exists(Path.Combine(other, "upload-test.txt")), "La tarea no utilizó la conexión personalizada.");
        var password = Guid.NewGuid().ToString("N"); await rclone.ProtectAsync(password); await rclone.LockAsync();
        var rejected = false; try { await rclone.ReadConfigAsync(); } catch (IOException) { rejected = true; } Check(rejected, "Se leyó una configuración bloqueada.");
        await rclone.UnlockAsync(password); loaded = await rclone.ReadConfigAsync();
        loaded = RcloneConfigDocument.Set(loaded, "teldrive", "custom_option", "cifrado"); await rclone.SaveConfigAsync(loaded);
        Check(rclone.Encrypted && (await rclone.ReadConfigAsync()).Contains("custom_option = cifrado"), "Se perdió el cifrado al editar.");
        await rclone.ConfigureAsync(new RcloneConnection("offline-test-api-key"));
        loaded = await rclone.ReadConfigAsync(); Check(loaded.Contains("[otra nube]") && loaded.Contains("custom_option = cifrado"), "Configurar Teldrive borró conexiones u opciones adicionales.");
        await runtime.StopAsync();
        await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "enhancements-smoke-test.json"), JsonSerializer.Serialize(new { success = true, navigation = true, activityScreenshot = "activity.png", importedOriginalPreserved = true, otherRemotesPreserved = true, customRemoteTransfer = true, backups = true, encryptedEditing = true, lockedEditingRejected = true, serverStopped = !runtime.Running }));
    }
    private static void Check(bool condition, string message) { if (!condition) throw new IOException(message); }
}
