using System.IO;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Imaging;

namespace Teldrive.Desktop;

internal static class RcloneAutomationSmokeTest
{
    public static async Task RunAsync(RuntimeSession runtime, MainWindow window)
    {
        var root = runtime.DataRoot;
        if (File.Exists(Path.Combine(root, "rclone.conf")) || File.Exists(Path.Combine(root, "rclone-automation.json"))) throw new IOException("La prueba requiere una carpeta nueva.");
        var remote = Path.Combine(root, "remote"); var source = Path.Combine(root, "source");
        Directory.CreateDirectory(remote); Directory.CreateDirectory(source);
        await File.WriteAllTextAsync(Path.Combine(source, "audio.txt"), "transferencia íntegra áéíóú");
        await File.WriteAllTextAsync(Path.Combine(source, "omitido.tmp"), "excluido");
        await File.WriteAllTextAsync(Path.Combine(root, "rclone.conf"), $"[teldrive]\ntype = alias\nremote = {remote}\n");
        await using var client = new RcloneRuntime(runtime);
        await using var engine = new RcloneAutomation(root, client);
        var task = new RcloneTaskDefinition { Name = "Copia con filtros", Source = source, Destination = "/copias", Excludes = "*.tmp", Bandwidth = "10M" };
        engine.SaveTask(task);
        engine.ConfigureQueue(true, 1);
        var first = engine.Enqueue(task);
        var preview = engine.Enqueue(task with { Destination = "/simulacion" }, true);
        engine.Move(preview.Id, -1); engine.Tick(DateTimeOffset.UtcNow);
        Check(engine.State.Queue[0].Id == preview.Id && first.State == "waiting", "La pausa o la prioridad fallaron.");
        engine.ConfigureQueue(false, 1); engine.Tick(DateTimeOffset.UtcNow);
        Check(preview.State == "running" && first.State == "waiting", "No se respetó la concurrencia.");
        await Wait(engine, preview);
        Check(!Directory.Exists(Path.Combine(remote, "simulacion")), "La simulación modificó el destino.");
        await Wait(engine, first);
        Check(File.Exists(Path.Combine(remote, "copias", "audio.txt")) && !File.Exists(Path.Combine(remote, "copias", "omitido.tmp")), "La copia o los filtros fallaron.");
        Check(await File.ReadAllTextAsync(Path.Combine(remote, "copias", "audio.txt")) == await File.ReadAllTextAsync(Path.Combine(source, "audio.txt")), "La copia cambió el contenido.");
        var download = engine.Enqueue(task with { Direction = "download", Source = "/copias", Destination = Path.Combine(root, "download") });
        await Wait(engine, download);
        Check(File.Exists(Path.Combine(root, "download", "audio.txt")), "La descarga no llegó.");

        var password = Guid.NewGuid().ToString("N"); await client.ProtectAsync(password); await client.LockAsync();
        var blocked = engine.Enqueue(task); engine.Tick(DateTimeOffset.UtcNow);
        Check(blocked.State == "waiting" && engine.BlockReason is not null, "La cola ignoró el bloqueo de rclone.");
        await client.UnlockAsync(password); await Wait(engine, blocked);
        var failure = engine.Enqueue(task with { Source = Path.Combine(root, "missing") }); engine.Tick(DateTimeOffset.UtcNow);
        Check(failure.State == "failed", "El origen inexistente no se informó.");
        Directory.CreateDirectory(failure.Definition.Source); await File.WriteAllTextAsync(Path.Combine(failure.Definition.Source, "retry.txt"), "reintento");
        engine.Retry(failure.Id); await Wait(engine, engine.State.Queue.Last());

        var unsafeTask = task with { Operation = "sync", Immutable = false, AllowDestructive = false };
        try { engine.Enqueue(unsafeTask); throw new InvalidOperationException("Se aceptó una sincronización sin autorización."); } catch (ArgumentException) { }
        var sync = engine.Enqueue(unsafeTask with { AllowDestructive = true, Destination = "/sync" }); await Wait(engine, sync);
        await File.WriteAllTextAsync(Path.Combine(remote, "sync", "extra.txt"), "sobrante");
        var sync2 = engine.Enqueue(sync.Definition); await Wait(engine, sync2);
        Check(!File.Exists(Path.Combine(remote, "sync", "extra.txt")), "La sincronización no replicó el origen.");

        var future = DateTimeOffset.UtcNow.AddMinutes(1);
        var schedule = new RcloneSchedule { TaskId = task.Id, Name = task.Name, NextRun = future, Repeat = "interval", IntervalMinutes = 1, RunMissed = true };
        engine.SaveSchedule(schedule); engine.ConfigureQueue(true, 1); engine.Tick(future);
        Check(engine.State.Queue.Last().State == "waiting" && schedule.NextRun > future, "El programador no encoló la tarea.");
        var queued = engine.State.Queue.Count; engine.Tick(future.AddMinutes(1));
        Check(engine.State.Queue.Count == queued && schedule.LastResult.Contains("pendiente"), "El programador superpuso una tarea pendiente.");
        var missed = new RcloneSchedule { TaskId = task.Id, Name = task.Name, NextRun = DateTimeOffset.UtcNow.AddMinutes(1), Repeat = "once", RunMissed = false };
        engine.SaveSchedule(missed); engine.Tick(future.AddHours(1));
        Check(!missed.Enabled && missed.LastResult.StartsWith("Horario omitido"), "El horario perdido no respetó su preferencia.");
        await engine.CancelAsync(engine.State.Queue.Last(item => item.State == "waiting").Id);
        engine.ConfigureQueue(false, 1);
        var large = Path.Combine(source, "large.dat"); await File.WriteAllBytesAsync(large, new byte[1024 * 1024]);
        var slow = engine.Enqueue(task with { Destination = "/slow", Bandwidth = "16K", Excludes = "" }); engine.Tick(DateTimeOffset.UtcNow);
        await Task.Delay(300); await engine.CancelAsync(slow.Id);
        Check(slow.State == "cancelled" && client.Snapshot(slow.ProcessId!)?.State != "running", "La cancelación dejó un proceso activo.");
        var interrupted = engine.Enqueue(task with { Destination = "/interrupted", Bandwidth = "16K", Excludes = "" }); engine.Tick(DateTimeOffset.UtcNow);
        await engine.DisposeAsync();
        await using var restored = new RcloneAutomation(root, client);
        Check(restored.State.Tasks.Count == 1 && restored.State.Schedules.Count == 2 && restored.State.Queue.Single(item => item.Id == interrupted.Id).State == "interrupted", "El reinicio perdió las tareas, horarios o estado de interrupción.");
        var prefs = new DesktopPreferences { AlwaysShowInTray = true, MinimizeToTray = true, CloseToTray = true, StartInTray = true }; prefs.Save(root);
        Check(DesktopPreferences.Load(root) == prefs, "No se conservaron las preferencias.");
        var startup = DesktopStartup.Command(@"C:\Program Files\Teldrive\Teldrive Desktop v2.exe", root, false);
        Check(startup.StartsWith('"') && startup.Contains("--start-in-tray --data-root \""), "El inicio automático no protege las rutas con espacios.");
        var portableStartup = DesktopStartup.Command(@"C:\Teldrive\Teldrive Desktop v2.exe", root, true);
        Check(portableStartup.EndsWith("--portable"), "El inicio automático perdió el modo portátil.");
        DesktopPreferences? chosenPreferences = null;
        var panel = new RcloneAdvancedView(); panel.Initialize(restored, prefs, value => chosenPreferences = value, () => { }, () => { }, client);
        panel.Measure(new Size(752, 430)); panel.Arrange(new Rect(0, 0, 752, 430));
        Check(panel.FindName("TasksGrid") is DataGrid && panel.FindName("QueueGrid") is DataGrid && panel.FindName("SchedulesGrid") is DataGrid, "Las pestañas no se crearon.");
        using (var tray = new DesktopTray(() => { }, () => { }, () => { }, () => { })) {
            tray.Apply(prefs, false); Check(tray.Visible, "La bandeja no mostró el icono.");
            tray.Apply(prefs with { AlwaysShowInTray = false }, false); Check(!tray.Visible, "No se aplicó la preferencia de visibilidad.");
            tray.HideWindowIcon(); Check(tray.Visible, "Ocultar la ventana no creó un icono recuperable."); tray.Restored(); Check(!tray.Visible, "Restaurar dejó un icono no solicitado.");
        }
        window.VerifyTrayLifecycle();
        ((TextBox)panel.FindName("TaskName")).Text = "Tarea creada desde el botón";
        ((TextBox)panel.FindName("Source")).Text = source;
        ((TextBox)panel.FindName("Destination")).Text = "/ui-test";
        ((Button)panel.FindName("SaveTaskButton")).RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
        Check(restored.State.Tasks.Any(item => item.Name == "Tarea creada desde el botón"), "El botón de guardar tarea no funcionó.");
        var tabs = (TabControl)panel.FindName("AdditionalTabs");
        #pragma warning disable WPF0001 // Match the light Fluent theme used by MainWindow.xaml.
        var previewWindow = new Window { Content = panel, Width = 1250, Height = 850, Opacity = 0, ShowInTaskbar = false, ShowActivated = false, ThemeMode = ThemeMode.Light };
        #pragma warning restore WPF0001
        previewWindow.Show();
        for (var index = 0; index < 4; index++) {
            tabs.SelectedIndex = index;
            await panel.Dispatcher.InvokeAsync(() => panel.UpdateLayout(), System.Windows.Threading.DispatcherPriority.ApplicationIdle);
            var bitmap = new RenderTargetBitmap((int)panel.ActualWidth, (int)panel.ActualHeight, 96, 96, PixelFormats.Pbgra32); bitmap.Render(panel);
            var png = new PngBitmapEncoder(); png.Frames.Add(BitmapFrame.Create(bitmap));
            using var stream = File.Create(Path.Combine(root, $"rclone-tab-{index}.png")); png.Save(stream);
        }
        previewWindow.Close();
        Check(((ComboBox)panel.FindName("LanguageChoice")).Items.Count == 10, "Faltan idiomas en Desktop.");
        foreach (var language in L.Languages) {
            L.Current.Language = language.Id;
            await panel.Dispatcher.InvokeAsync(() => { }, System.Windows.Threading.DispatcherPriority.ApplicationIdle);
            Check((string)((Button)panel.FindName("SaveTaskButton")).Content == L.T("Guardar tarea"), "El botón no actualizó su traducción.");
        }
        ((ComboBox)panel.FindName("LanguageChoice")).SelectedValue = "en";
        ((Button)panel.FindName("SavePreferencesButton")).RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
        Check(chosenPreferences?.Language == "en", "No se guardó el idioma elegido en Preferencias.");
        L.Current.Language = "es";
        ((ComboBox)panel.FindName("Operation")).SelectedIndex = 4;
        ((TextBox)panel.FindName("TaskName")).Text = "Montaje creado desde el botón";
        ((TextBox)panel.FindName("Source")).Text = "/";
        ((TextBox)panel.FindName("Destination")).Text = "Y:";
        ((CheckBox)panel.FindName("MountStartup")).IsChecked = true;
        ((Button)panel.FindName("SaveTaskButton")).RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
        Check(restored.State.Tasks.Any(item => item.IsMount && item.RunAtStartup && item.Destination == "Y:"), "El editor no guardó el montaje persistente.");
        var json = await File.ReadAllTextAsync(Path.Combine(root, "rclone-automation.json"));
        Check(!json.Contains(password), "Las tareas guardaron una contraseña.");
        await PersistentMountSmokeTest.RunAsync(runtime);
        await File.WriteAllTextAsync(Path.Combine(root, "automation-smoke-test.json"), JsonSerializer.Serialize(new { success = true, version = "2.0.0.6", scenarios = new[] { "real-rclone-copy", "filters", "download", "dry-run", "pause", "priority", "concurrency", "encrypted-lock", "failure-retry", "sync-consent", "sync-delete", "schedule", "duplicate-suppression", "missed-run", "cancellation", "restart-persistence", "preferences", "startup-quoting", "portable-startup", "wpf-controls", "save-task-button", "tray-icon", "minimize-to-tray", "restore-from-tray", "close-to-tray", "no-stored-password", "persistent-winfsp-mount", "startup-server-wait", "startup-unlock-wait", "mount-restart", "mount-transfer-concurrency", "mount-cancellation", "ten-native-languages", "language-preference-button", "mount-task-editor-button" } }));
    }
    private static void Check(bool success, string error) { if (!success) throw new IOException(error); }
    private static async Task Wait(RcloneAutomation engine, RcloneQueueItem item)
    {
        for (var attempt = 0; attempt < 300; attempt++) {
            engine.Tick(DateTimeOffset.UtcNow);
            if (item.State == "completed") return;
            if (item.State is "failed" or "cancelled" or "interrupted") throw new IOException(item.Log);
            await Task.Delay(100);
        }
        throw new TimeoutException("La tarea de prueba no terminó.");
    }
}
