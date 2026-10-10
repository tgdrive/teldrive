using System.IO;
using System.Text.Json;

namespace Teldrive.Desktop;

// Exercises the real bundled executable against an isolated local alias remote.
// Telegram transport is tested separately by the backend integration suite.
internal static class RcloneSmokeTest
{
    public static async Task RunAsync(RuntimeSession runtime)
    {
        if (File.Exists(Path.Combine(runtime.DataRoot, "rclone.conf"))) throw new IOException("La prueba requiere una carpeta nueva y vacía para evitar modificar una conexión existente.");
        var remote = Path.Combine(runtime.DataRoot, "smoke-remote");
        Directory.CreateDirectory(remote);
        await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "rclone.conf"), $"[teldrive]\ntype = alias\nremote = {remote}\n");
        await using var rclone = new RcloneRuntime(runtime);
        var secret = Guid.NewGuid().ToString("N");
        await rclone.ProtectAsync(secret);
        if (!rclone.Encrypted || rclone.Locked) throw new IOException("La configuración no quedó cifrada y desbloqueada.");
        await rclone.LockAsync();
        if (!rclone.Locked) throw new IOException("La configuración no se bloqueó.");
        try { await rclone.UnlockAsync("incorrect-password"); throw new InvalidOperationException("Se aceptó una contraseña incorrecta."); } catch (IOException) {}
        await rclone.UnlockAsync(secret);
        await rclone.CreateFolderAsync("/carpeta");
        var source = Path.Combine(runtime.DataRoot, "prueba.txt");
        const string contents = "Teldrive: transferencia íntegra áéíóú ☁️";
        await File.WriteAllTextAsync(source, contents);
        var upload = rclone.Upload(source, "/carpeta"); await WaitAsync(rclone, upload);
        await rclone.RenameAsync("/carpeta/prueba.txt", "/carpeta/renombrado.txt");
        var listing = await rclone.BrowseAsync("/carpeta");
        if (listing.GetArrayLength() != 1 || listing[0].GetProperty("Name").GetString() != "renombrado.txt") throw new IOException("El explorador no devolvió el archivo renombrado.");
        var size = await rclone.SizeAsync("/carpeta");
        if (size.GetProperty("count").GetInt32() != 1) throw new IOException("El cálculo de tamaño no devolvió un archivo.");
        var destination = Path.Combine(runtime.DataRoot, "descargas"); Directory.CreateDirectory(destination);
        var download = rclone.Download("/carpeta/renombrado.txt", destination); await WaitAsync(rclone, download);
        if (await File.ReadAllTextAsync(Path.Combine(destination, "renombrado.txt")) != contents) throw new IOException("El contenido de la descarga cambió.");
        await WaitAsync(rclone, rclone.Retry(download));
        var wav = Path.Combine(remote, "audio.wav");
        using (var file = File.Create(wav)) using (var writer = new BinaryWriter(file)) {
            const int sampleRate = 22050, samples = 6615;
            writer.Write("RIFF"u8); writer.Write(36 + samples * 2); writer.Write("WAVEfmt "u8); writer.Write(16); writer.Write((short)1); writer.Write((short)1); writer.Write(sampleRate); writer.Write(sampleRate * 2); writer.Write((short)2); writer.Write((short)16); writer.Write("data"u8); writer.Write(samples * 2);
            for (var i = 0; i < samples; i++) writer.Write((short)(Math.Sin(2 * Math.PI * 440 * i / sampleRate) * 1000));
        }
        await using (var media = new MediaSession(runtime)) await media.StartAsync(rclone, "/audio.wav", 0, headless:true);
        var localFolder = Path.Combine(runtime.DataRoot, "lote"); Directory.CreateDirectory(localFolder);
        await File.WriteAllTextAsync(Path.Combine(localFolder, "segundo.txt"), contents);
        await WaitAsync(rclone, rclone.UploadFolder(localFolder, "/"));
        if (!File.Exists(Path.Combine(remote, "lote", "segundo.txt"))) throw new IOException("La subida de carpetas no terminó.");
        await rclone.TrashAsync("/carpeta", true);
        if (Directory.Exists(Path.Combine(remote, "carpeta"))) throw new IOException("La eliminación de la carpeta no terminó.");
        var mounted = false;
        if (rclone.DriverInstalled) {
            var occupied = DriveInfo.GetDrives().Select(drive => drive.Name[..2]).ToHashSet(StringComparer.OrdinalIgnoreCase);
            var letter = Enumerable.Range('D', 23).Reverse().Select(value => ((char)value) + ":").FirstOrDefault(value => !occupied.Contains(value));
            if (letter is null) throw new IOException("No hay una letra libre para la prueba de montaje.");
            var mount = rclone.Mount(letter);
            try {
                var driveRoot = letter + Path.DirectorySeparatorChar;
                for (var attempt = 0; attempt < 100 && !File.Exists(Path.Combine(driveRoot, "audio.wav")); attempt++) await Task.Delay(200);
                if (!File.Exists(Path.Combine(driveRoot, "audio.wav"))) throw new IOException("La unidad de prueba no se montó.");
                await File.WriteAllTextAsync(Path.Combine(driveRoot, "montaje.txt"), contents);
                if (await File.ReadAllTextAsync(Path.Combine(driveRoot, "montaje.txt")) != contents) throw new IOException("La lectura de la unidad montada falló.");
                for (var attempt = 0; attempt < 100 && !File.Exists(Path.Combine(remote, "montaje.txt")); attempt++) await Task.Delay(200);
                if (!File.Exists(Path.Combine(remote, "montaje.txt")) || await File.ReadAllTextAsync(Path.Combine(remote, "montaje.txt")) != contents) throw new IOException("La unidad no guardó el archivo remoto.");
                mounted = true;
            } finally { await rclone.StopAsync(mount); }
        }
        if (File.Exists(Path.Combine(runtime.DataRoot, "rclone.conf.encrypted.old"))) throw new IOException("Quedó una copia sin cifrar de la configuración.");
        await rclone.LockAsync();
        await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "rclone-smoke-test.json"), JsonSerializer.Serialize(new {success = true, encrypted = rclone.Encrypted, locked = rclone.Locked, mounted, scenarios = new[] {"encryption", "wrong-password", "unlock", "mkdir", "upload", "rename", "browse", "size", "download", "retry", "stream-audio-mpv", "folder-upload", "delete", "lock"}}));
    }
    private static async Task WaitAsync(RcloneRuntime rclone, string id)
    {
        for (var attempt = 0; attempt < 150; attempt++) {
            var tasks = JsonSerializer.SerializeToElement(rclone.List());
            var task = tasks.EnumerateArray().Single(item => item.GetProperty("id").GetString() == id);
            var status = task.GetProperty("status").GetString();
            if (status == "completed") return;
            if (status is "failed" or "cancelled") throw new IOException(task.GetProperty("log").GetString());
            await Task.Delay(200);
        }
        throw new TimeoutException("La transferencia de prueba no terminó.");
    }
}
