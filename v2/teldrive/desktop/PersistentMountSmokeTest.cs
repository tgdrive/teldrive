using System.IO;
using System.Text.Json;

namespace Teldrive.Desktop;

internal static class PersistentMountSmokeTest
{
    public static async Task RunAsync(RuntimeSession parent)
    {
        var root = Path.Combine(parent.DataRoot, "mount-test");
        await using var runtime = new RuntimeSession(root);
        // Use the parent's verified, immutable payload; all writable test data is isolated.
        typeof(RuntimeSession).GetProperty(nameof(RuntimeSession.Components))!.SetValue(runtime, parent.Components);
        var remote = Path.Combine(root, "remote"); Directory.CreateDirectory(remote);
        await File.WriteAllTextAsync(Path.Combine(remote, "mounted.txt"), "persistent mount proof");
        await File.WriteAllTextAsync(Path.Combine(root, "rclone.conf"), $"[teldrive]\ntype = alias\nremote = {remote}\n");
        await using var client = new RcloneRuntime(runtime);
        var ready = false; client.AutomationReady = () => ready;
        var letter = Enumerable.Range('D', 23).Reverse().Select(value => ((char)value) + ":").First(value => !DriveInfo.GetDrives().Any(drive => drive.Name.StartsWith(value, StringComparison.OrdinalIgnoreCase)));
        var mount = new RcloneTaskDefinition { Name = "Montaje persistente de prueba", Operation = "mount", Source = "/", Destination = letter, RunAtStartup = true, MountReadOnly = true };
        await using (var setup = new RcloneAutomation(root, client)) {
            setup.SaveTask(mount);
            try { setup.SaveTask(mount with { Id = Guid.NewGuid().ToString("N") }); throw new IOException("Se aceptaron dos montajes al iniciar con la misma letra."); } catch (ArgumentException) { }
        }
        await using (var first = new RcloneAutomation(root, client)) {
            first.Tick(DateTimeOffset.UtcNow); first.Tick(DateTimeOffset.UtcNow);
            Check(first.State.Queue.Count(item => item.State == "waiting") == 1, "Se duplicó la tarea al iniciar o no esperó al servidor.");
            ready = true; first.Tick(DateTimeOffset.UtcNow);
            if (!client.DriverInstalled) throw new IOException("La prueba real requiere WinFsp.");
            var job = first.State.Queue.Single(); await Mounted(client, job, letter);
            var copy = first.Enqueue(new() { Name = "Copia con unidad montada", Source = remote, Destination = "/copy", Excludes = "/copy/**" });
            for (var attempt = 0; attempt < 100 && copy.State is "waiting" or "running"; attempt++) { first.Tick(DateTimeOffset.UtcNow); await Task.Delay(100); }
            Check(copy.State == "completed" && job.State == "running", "El montaje bloqueó la cola de transferencias.");
            await first.CancelAsync(job.Id); await Unmounted(letter);
            Check(job.State == "cancelled", "No se desmontó la tarea cancelada.");
        }
        var password = Guid.NewGuid().ToString("N"); await client.ProtectAsync(password); await client.LockAsync();
        await using (var restarted = new RcloneAutomation(root, client)) {
            restarted.Tick(DateTimeOffset.UtcNow);
            var job = restarted.State.Queue.Last();
            Check(job.State == "waiting" && client.Locked, "El montaje ignoró la configuración cifrada.");
            await client.UnlockAsync(password); restarted.Tick(DateTimeOffset.UtcNow); await Mounted(client, job, letter);
            Check(restarted.State.Queue.Count(item => item.State == "running") == 1, "El reinicio duplicó el montaje.");
            await restarted.CancelAsync(job.Id); await Unmounted(letter);
        }
        Check(!File.ReadAllText(Path.Combine(root, "rclone-automation.json")).Contains(password), "Se guardó una contraseña en las tareas.");
        await File.WriteAllTextAsync(Path.Combine(parent.DataRoot, "persistent-mount-smoke-test.json"), JsonSerializer.Serialize(new { success = true, realWinFspMount = true, startupWaitsForServer = true, startupWaitsForUnlock = true, restartRemounts = true, noDuplicates = true, transferQueueContinues = true, cancellationUnmounts = true, noStoredPassword = true }));
    }
    private static void Check(bool result, string message) { if (!result) throw new IOException(message); }
    private static async Task Mounted(RcloneRuntime client, RcloneQueueItem job, string letter)
    {
        for (var attempt = 0; attempt < 100; attempt++) {
            if (File.Exists(letter + "\\mounted.txt")) return;
            var status = client.Snapshot(job.ProcessId!);
            if (status?.State != "running") throw new IOException(status?.Log ?? job.Log);
            await Task.Delay(100);
        }
        throw new IOException("WinFsp no creó la unidad de prueba.");
    }
    private static async Task Unmounted(string letter)
    {
        for (var attempt = 0; attempt < 100; attempt++) {
            if (!DriveInfo.GetDrives().Any(item => item.Name.StartsWith(letter, StringComparison.OrdinalIgnoreCase))) return;
            await Task.Delay(100);
        }
        throw new IOException("La unidad de prueba quedó montada.");
    }
}
