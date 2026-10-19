using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Text.Json;
using System.Windows;
using System.Windows.Threading;
using Microsoft.Web.WebView2.Core;

namespace Teldrive.Desktop;

public partial class MainWindow : Window
{
    private readonly WindowFullscreen browserFullscreen;
    private readonly RuntimeSession runtime;
    private readonly RcloneRuntime rclone;
    private readonly List<MediaWindow> players = [];
    private async Task StopPlayersAsync() { foreach (var player in players.ToArray()) await player.StopAsync(); }
    private string? localFolder;
    private Task? currentOperation;
    private bool busy;
    private bool closing;
    private bool closed;
    private readonly string[] arguments;
    private readonly bool portable;
    private readonly bool smoke;
    private DesktopPreferences preferences = new();
    private DesktopTray? tray;
    private RcloneAutomation? automation;
    private readonly DispatcherTimer automationTimer = new() { Interval = TimeSpan.FromSeconds(1) };
    private bool exitRequested;
    private bool testingTray;
    private bool requestedStartHidden;
    public MainWindow(string[] args)
    {
        InitializeComponent(); browserFullscreen = new WindowFullscreen(this); arguments = args;
        smoke = args.Any(arg => arg.StartsWith("--smoke-", StringComparison.Ordinal));
        if (smoke) { Opacity = 0; ShowInTaskbar = false; ShowActivated = false; }
        var rootIndex = Array.IndexOf(args, "--data-root");
        portable = args.Contains("--portable") || File.Exists(Path.Combine(AppContext.BaseDirectory, "Teldrive.portable"));
        runtime = new RuntimeSession(rootIndex >= 0 && rootIndex + 1 < args.Length ? args[rootIndex + 1] : portable ? Path.Combine(AppContext.BaseDirectory, "TeldriveData") : null);
        rclone = new RcloneRuntime(runtime);
        rclone.AutomationReady = () => runtime.Running && !busy;
        StateChanged += (_, _) => { if ((!smoke || testingTray) && WindowState == WindowState.Minimized && preferences.MinimizeToTray) HideToTray(); };
        Application.Current.SessionEnding += (_, _) => { exitRequested = true; automationTimer.Stop(); };
        automationTimer.Tick += (_, _) => {
            try {
                automation?.Tick(DateTimeOffset.UtcNow);
                if (AdvancedView.IsVisible) AdvancedView.Refresh();
                if (automation is not null) tray?.QueueState(automation.State.Paused, automation.State.Queue.Count(item => item.State is "waiting" or "running"));
            } catch (Exception error) { automationTimer.Stop(); Status.Text = L.T("Programador detenido: ") + error.Message; RestoreFromTray(); }
        };
        runtime.Activity += line => Dispatcher.BeginInvoke(() => { if (Logs.Text.Length > 200000) Logs.Text = Logs.Text[^100000..]; Logs.AppendText(line + "\n"); Logs.ScrollToEnd(); });
        Loaded += async (_, _) => await Guard(async () => {
            var startupSmoke = args.Contains("--smoke-startup-tray");
            if (!smoke || startupSmoke) {
                InitializeAutomation();
                requestedStartHidden = (preferences.StartInTray || args.Contains("--start-in-tray")) && runtime.State is not null && File.Exists(runtime.ConfigPath);
                if (requestedStartHidden) { Opacity = 0; ShowInTaskbar = false; ShowActivated = false; tray!.HideWindowIcon(); }
            }
            await runtime.PrepareAsync();
            if (args.Contains("--smoke-input")) {
                var listener = new System.Net.Sockets.TcpListener(System.Net.IPAddress.Loopback, 0); listener.Start(); var port = ((System.Net.IPEndPoint)listener.LocalEndpoint).Port; listener.Stop();
                await runtime.ConfigureAsync(port, true, "", ""); await VerifyFolderInputAsync(); await runtime.StopAsync(); Close(); return;
            }
            if (args.Contains("--smoke-player")) { await PlayerSmokeTest.RunAsync(runtime); Close(); return; }
            if (args.Contains("--smoke-enhancements")) { await DesktopEnhancementSmoke.RunAsync(runtime, this); Close(); return; }
            if (args.Contains("--smoke-automation")) { await RcloneAutomationSmokeTest.RunAsync(runtime, this); Close(); return; }
            if (args.Contains("--smoke-rclone")) { await RcloneSmokeTest.RunAsync(runtime); Close(); return; }
            if (args.Contains("--smoke-server")) {
                if (runtime.State is not null || File.Exists(runtime.ConfigPath)) throw new IOException(L.T("La prueba del servidor requiere una carpeta nueva."));
                var probe = new System.Net.Sockets.TcpListener(System.Net.IPAddress.Loopback, 0); probe.Start(); var port = ((System.Net.IPEndPoint)probe.LocalEndpoint).Port; probe.Stop();
                await runtime.ConfigureAsync(port, true, "", ""); await ShowDrive();
                var rendered = false;
                for (var attempt = 0; attempt < 60; attempt++) {
                    var value = await Browser.CoreWebView2.ExecuteScriptAsync("JSON.stringify({title:document.title, text:document.body.innerText})");
                    var json = JsonSerializer.Deserialize<string>(value);
                    if (json is not null && json.Contains(L.T("Iniciar sesión"), StringComparison.OrdinalIgnoreCase)) { rendered = true; break; }
                    await Task.Delay(500);
                }
                if (!rendered) throw new IOException(L.T("El servidor respondió, pero la pantalla de inicio de sesión no se mostró."));
                await Browser.CoreWebView2.CallDevToolsProtocolMethodAsync("Runtime.evaluate", JsonSerializer.Serialize(new {expression="document.documentElement.requestFullscreen()",userGesture=true,awaitPromise=true}));
                await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
                if(!browserFullscreen.Active||WindowStyle!=WindowStyle.None||AppHeader.Visibility!=Visibility.Collapsed)throw new IOException("El vídeo web no activó la pantalla completa de Desktop.");
                await Browser.CoreWebView2.ExecuteScriptAsync("document.exitFullscreen()");
                await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
                if(browserFullscreen.Active||AppHeader.Visibility!=Visibility.Visible)throw new IOException("La pantalla completa web no restauró Desktop.");
                InitializeAutomation();
                tray!.InvokeMenuForSmoke(0);tray.InvokeMenuForSmoke(1);tray.InvokeMenuForSmoke(2);
                await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
                if(AdvancedView.Visibility!=Visibility.Visible||!automation!.State.Paused)throw new IOException("El menú de bandeja falló con WebView2 inicializado.");
                var originalConfig = await File.ReadAllTextAsync(runtime.ConfigPath);
                var rejected = false;
                try { await runtime.SaveConfigAsync(originalConfig.Replace($"127.0.0.1:{port}", $"203.0.113.2:{port}")); }
                catch (IOException) { rejected = true; }
                if (!rejected || await File.ReadAllTextAsync(runtime.ConfigPath) != originalConfig || runtime.ServerUri!.Port != port) throw new IOException(L.T("Una dirección no local modificó la configuración válida."));
                await runtime.StopAsync();
                await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "server-smoke-test.json"), JsonSerializer.Serialize(new {success=true, postgresql=true, migrations=true, loginUi=true, rejectedNonLocalAddress=rejected, browserFullscreen=true, trayWithWebView=true, stopped=!runtime.Running, port}));
                Close(); return;
            }
            RefreshConfiguration();
            Status.Text = L.T("Listo para configurar tu unidad");
            if (runtime.State is not null && File.Exists(runtime.ConfigPath)) { await runtime.StartAsync(); await ShowDrive(); }
            if ((!smoke || startupSmoke) && requestedStartHidden && runtime.Running) { HideToTray(); if (!smoke) Opacity = 1; }
            if (startupSmoke) {
                if (!runtime.Running || IsVisible || tray?.Visible != true) throw new IOException(L.T("El inicio automático no quedó activo en la bandeja."));
                testingTray = true; RestoreFromTray(); testingTray = false;
                if (!IsVisible || WindowState != WindowState.Normal) throw new IOException(L.T("La ventana iniciada en la bandeja no se pudo recuperar."));
                await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "startup-tray-smoke-test.json"), JsonSerializer.Serialize(new { success = true, serverRunning = runtime.Running, startedHidden = true, restored = true, windowsStartupRegistryModified = false }));
                exitRequested = true; Close(); return;
            }
            if (args.Contains("--smoke-test")) {
                await InitializeBrowser();
                await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "smoke-test.json"), JsonSerializer.Serialize(new {success = true, webview = Browser.CoreWebView2.Environment.BrowserVersionString, runtime = Environment.Version.ToString(), components = runtime.Components}));
                Close();
            }
        });
        Closing += async (_, e) => {
            if (closed) return;
            e.Cancel = true;
            if (closing) return;
            if ((!smoke || testingTray) && !exitRequested && preferences.CloseToTray && tray is not null) { HideToTray(); return; }
            closing = true;
            automationTimer.Stop();
            try {
                if (currentOperation is not null) { try { await currentOperation; } catch { /* Guard already reports the operation failure; cleanup must still run. */ } }
                if (automation is not null) await automation.DisposeAsync();
                void ShutdownStage(string stage) { if (smoke) File.AppendAllText(Path.Combine(runtime.DataRoot, "shutdown-smoke.log"), stage + "\n"); }
                ShutdownStage("players"); await StopPlayersAsync();
                ShutdownStage("rclone"); await rclone.DisposeAsync();
                ShutdownStage("runtime"); await runtime.DisposeAsync();
                ShutdownStage("tray"); tray?.Dispose();
                ShutdownStage("browser"); Browser.Dispose();
                // Finish the current Closing event before requesting application shutdown.
                // An already-stopped runtime can complete cleanup synchronously.
                ShutdownStage("application"); closed = true;
                _ = Dispatcher.BeginInvoke(() => Application.Current.Shutdown());
            }
            catch (Exception error) { closing = false; MessageBox.Show(error.Message, L.T("No se pudieron detener los servicios")); }
        };
    }
    private async Task Guard(Func<Task> operation)
    {
        if (busy) return;
        busy = true; StartButton.IsEnabled = false; StopButton.IsEnabled = false;
        try { currentOperation = operation(); await currentOperation; }
        catch (Exception error) {
            Status.Text = error.Message;
            if (smoke) { await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "smoke-test.json"), JsonSerializer.Serialize(new {success = false, error = error.ToString()})); Close(); }
            else { RestoreFromTray(); MessageBox.Show(error.Message, L.T("Teldrive Desktop v2"), MessageBoxButton.OK, MessageBoxImage.Information); }
        }
        finally { currentOperation = null; busy = false; StartButton.IsEnabled = !runtime.Running; StopButton.IsEnabled = runtime.Running; }
    }
    private void RefreshConfiguration()
    {
        if (File.Exists(runtime.ConfigPath)) ConfigEditor.Text = File.ReadAllText(runtime.ConfigPath);
        if (runtime.State is not null) { HttpPort.Text = runtime.State.HttpPort.ToString(); LocalDatabase.IsChecked = runtime.State.LocalDatabase; DatabaseUrl.Text = runtime.State.DatabaseUrl; AllowedUser.Text = runtime.State.AllowedUser; }
    }
    private async Task InitializeBrowser()
    {
        if (Browser.CoreWebView2 is not null) return;
        // Composition initialization needs a measured visual surface. Preparing
        // it while collapsed leaves EnsureCoreWebView2Async waiting indefinitely.
        Browser.Visibility = Visibility.Visible;
        Browser.UpdateLayout();
        await Dispatcher.Yield(DispatcherPriority.Loaded);
        try { _ = CoreWebView2Environment.GetAvailableBrowserVersionString(); }
        catch (WebView2RuntimeNotFoundException) {
            Status.Text = L.T("Preparando WebView2 integrado…");
            await runtime.InstallAsync("WebView2RuntimeInstaller.exe", false, "/silent", "/install");
        }
        var environment = await CoreWebView2Environment.CreateAsync(userDataFolder: Path.Combine(runtime.DataRoot, "webview"));
        await Browser.EnsureCoreWebView2Async(environment);
        if (Browser.CoreWebView2 is null) throw new IOException(L.T("No se pudo inicializar WebView2."));
        Browser.CoreWebView2.Settings.IsStatusBarEnabled = false;
        Browser.CoreWebView2.Settings.AreDevToolsEnabled = false;
        await Browser.CoreWebView2.AddScriptToExecuteOnDocumentCreatedAsync($"if (location.protocol === 'http:' && location.hostname === '127.0.0.1' && !localStorage.getItem('teldrive-language')) localStorage.setItem('teldrive-language', {JsonSerializer.Serialize(preferences.Language)});");
        Browser.CoreWebView2.ContainsFullScreenElementChanged += (_, _) => {
            var active=Browser.CoreWebView2.ContainsFullScreenElement;
            browserFullscreen.Set(active);
            AppHeader.Visibility=AppFooter.Visibility=active?Visibility.Collapsed:Visibility.Visible;
            AppShell.RowDefinitions[0].MinHeight=active?0:64;
            AppShell.RowDefinitions[0].Height=active?new GridLength(0):GridLength.Auto;
            AppShell.RowDefinitions[2].Height=new GridLength(active?0:34);
        };
        Browser.CoreWebView2.WebMessageReceived += ReceiveNativeMessage;
        Browser.CoreWebView2.NavigationStarting += (_, e) => {
            if (runtime.ServerUri is null || !Uri.TryCreate(e.Uri, UriKind.Absolute, out var uri)) { e.Cancel = true; return; }
            if (uri.Scheme == "blob" && e.Uri.StartsWith("blob:" + runtime.ServerUri.GetLeftPart(UriPartial.Authority), StringComparison.Ordinal)) return;
            if (uri.GetLeftPart(UriPartial.Authority) != runtime.ServerUri.GetLeftPart(UriPartial.Authority)) { e.Cancel = true; if (e.IsUserInitiated && uri.Scheme is "https" or "http") OpenExternal(uri.AbsoluteUri); }
        };
        Browser.CoreWebView2.NewWindowRequested += (_, e) => { e.Handled = true; if (e.IsUserInitiated && Uri.TryCreate(e.Uri, UriKind.Absolute, out var uri) && uri.Scheme is "https" or "http") OpenExternal(uri.AbsoluteUri); };
    }
    private async Task ShowDrive(string path = "/")
    {
        AdvancedView.Visibility = Visibility.Collapsed;
        if (!runtime.Running) { SetupView.Visibility = Visibility.Visible; Browser.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; return; }
        await InitializeBrowser();
        SetupView.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; Browser.Visibility = Visibility.Visible;
        Browser.Source = new Uri(runtime.ServerUri!, path);
        Status.Text = L.T("Conectado \u00B7 {0} \u00B7 Datos guardados en tu equipo", runtime.ServerUri);
    }
    private async void StartClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.StartAsync(); await ShowDrive(); });
    private async void StopClick(object sender, RoutedEventArgs e) => await Guard(async () => { if (automation is not null) automation.ConfigureQueue(true, automation.State.ParallelJobs); await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.StopAsync(); AdvancedView.Visibility = Visibility.Collapsed; Browser.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Visible; Status.Text = L.T("Servicios detenidos; cola pausada"); });
    private void SetupClick(object sender, RoutedEventArgs e) { AdvancedView.Visibility = Visibility.Collapsed; RefreshConfiguration(); Browser.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Visible; }
    private void LogsClick(object sender, RoutedEventArgs e) { AdvancedView.Visibility = Visibility.Collapsed; Browser.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Visible; }
    private void AdvancedClick(object sender, RoutedEventArgs e) => ShowAdvanced();
    private void ShowAdvanced() { Browser.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; AdvancedView.Visibility = Visibility.Visible; AdvancedView.Refresh(); }
    private void InitializeAutomation()
    {
        preferences = DesktopPreferences.Load(runtime.DataRoot);
        ApplyLanguage(preferences.Language);
        automation = new RcloneAutomation(runtime.DataRoot, rclone);
        tray = new DesktopTray(RestoreFromTray, () => { RestoreFromTray(); ShowAdvanced(); }, () => { automation.ConfigureQueue(!automation.State.Paused, automation.State.ParallelJobs); AdvancedView.Refresh(); }, ExitDesktop);
        tray.Apply(preferences, false);
        automation.Finished += item => { if (preferences.NotifyFinished) tray.Notify(item); };
        AdvancedView.Initialize(automation, preferences, SavePreferences, HideToTray, ExitDesktop, rclone);
        automationTimer.Start();
    }
    private void SavePreferences(DesktopPreferences value)
    {
        DesktopStartup.Apply(value.StartWithWindows, runtime.DataRoot, portable);
        value.Save(runtime.DataRoot); preferences = value; tray?.Apply(value, !IsVisible);
        ApplyLanguage(value.Language);
        if (Browser.CoreWebView2 is not null) _ = Browser.CoreWebView2.ExecuteScriptAsync($"localStorage.setItem('teldrive-language', {JsonSerializer.Serialize(value.Language)}); location.reload();");
    }
    private void ApplyLanguage(string language)
    {
        L.Current.Language = language;
        FlowDirection = language == "ar" ? FlowDirection.RightToLeft : FlowDirection.LeftToRight;
        foreach (var field in new[] { ConfigEditor, DatabaseUrl, HttpPort, AllowedUser, Logs }) field.FlowDirection = FlowDirection.LeftToRight;
        Browser.FlowDirection = FlowDirection.LeftToRight;
    }
    internal void HideToTray()
    {
        if (tray is null) return;
        tray.HideWindowIcon(); Hide(); ShowInTaskbar = false;
    }
    internal void RestoreFromTray()
    {
        if ((smoke && !testingTray) || closed || closing) return;
        requestedStartHidden = false; Opacity = smoke ? 0 : 1;
        ShowInTaskbar = !smoke; Show(); WindowState = WindowState.Normal; if (!smoke) Activate(); tray?.Restored();
    }
    internal void VerifyTrayLifecycle()
    {
        testingTray = true;
        preferences = new() { MinimizeToTray = true, CloseToTray = true };
        tray = new DesktopTray(RestoreFromTray, ShowAdvanced, () => { }, ExitDesktop);
        try {
            WindowState = WindowState.Minimized;
            if (IsVisible || !tray.Visible) throw new IOException(L.T("Minimizar no ocultó la ventana en la bandeja."));
            RestoreFromTray();
            if (!IsVisible || WindowState != WindowState.Normal) throw new IOException(L.T("La bandeja no recuperó la ventana."));
            Close();
            if (IsVisible || !tray.Visible || closing) throw new IOException(L.T("Cerrar no conservó el programa en la bandeja."));
            RestoreFromTray();
        } finally { preferences = new(); tray.Dispose(); tray = null; testingTray = false; }
    }
    private void ExitDesktop() { exitRequested = true; Close(); }
    private async void DriveClick(object sender, RoutedEventArgs e) => await Guard(() => ShowDrive());
    private async void RcloneClick(object sender, RoutedEventArgs e) => await Guard(() => ShowDrive("/settings/rclone"));
    private async void ConfigureClick(object sender, RoutedEventArgs e) => await Guard(async () => {
        if (!int.TryParse(HttpPort.Text, out var port)) throw new ArgumentException(L.T("Introduce un puerto válido."));
        Status.Text = L.T("Preparando la base de datos y el servidor…");
        await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.ConfigureAsync(port, LocalDatabase.IsChecked == true, DatabaseUrl.Text.Trim(), AllowedUser.Text); RefreshConfiguration(); await ShowDrive();
    });
    private async void SaveConfigClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.SaveConfigAsync(ConfigEditor.Text); await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.StopAsync(); await runtime.StartAsync(); await ShowDrive(); });
    private async void ValidateClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.ValidateConfigAsync(ConfigEditor.Text); Status.Text = L.T("Configuración válida"); });
    private async void WinFspClick(object sender, RoutedEventArgs e) => await Guard(() => runtime.InstallAsync("winfsp-2.1.25156.msi", true));
    private async void RuntimeClick(object sender, RoutedEventArgs e) => await Guard(() => runtime.InstallAsync("VC_redist.x64.exe", false, "/install", "/passive", "/norestart"));
    private void DataClick(object sender, RoutedEventArgs e) => OpenExternal(runtime.DataRoot);
    private static void OpenExternal(string target) => Process.Start(new ProcessStartInfo(target) { UseShellExecute = true });
    private async void ReceiveNativeMessage(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        if (closing || !runtime.Running || runtime.ServerUri is null || !Uri.TryCreate(e.Source, UriKind.Absolute, out var source)
            || source.GetLeftPart(UriPartial.Authority) != runtime.ServerUri.GetLeftPart(UriPartial.Authority)) return;
        string? id = null;
        try {
            if (e.WebMessageAsJson.Length > 4 * 1024 * 1024) throw new IOException(L.T("La solicitud es demasiado grande."));
            using var message = JsonDocument.Parse(e.WebMessageAsJson);
            id = message.RootElement.GetProperty("id").GetString();
            if (id is null || !Guid.TryParse(id, out _)) return;
            var method = message.RootElement.GetProperty("method").GetString();
            if (method != "casting.open-browser" && source.AbsolutePath is not ("/settings/rclone" or "/settings/server")) return;
            await RequireOwnerAsync();
            var parameters = message.RootElement.GetProperty("params");
            string Get(string key) => parameters.GetProperty(key).GetString() ?? "";
            object? result;
            switch (method) {
                case "casting.open-browser":
                    if (!Uri.TryCreate(Get("url"), UriKind.Absolute, out var castingUrl) || castingUrl.GetLeftPart(UriPartial.Authority) != runtime.ServerUri.GetLeftPart(UriPartial.Authority) || castingUrl.AbsolutePath != "/cast") throw new ArgumentException(L.T("Enlace de transmisión no válido."));
                    var chrome = new[] { Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles), L.T("Google"), L.T("Chrome"), L.T("Application"), "chrome.exe"), Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86), L.T("Google"), L.T("Chrome"), L.T("Application"), "chrome.exe"), Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData), L.T("Google"), L.T("Chrome"), L.T("Application"), "chrome.exe") }.FirstOrDefault(File.Exists);
                    if (chrome is null) throw new IOException(L.T("Instala Chrome para usar Chromecast o abre el enlace en un navegador compatible."));
                    var castBrowser = new ProcessStartInfo(chrome) { UseShellExecute = false }; castBrowser.ArgumentList.Add(castingUrl.AbsoluteUri); Process.Start(castBrowser); result = true; break;
                case "rclone.config-import":
                    var configPicker = new Microsoft.Win32.OpenFileDialog { Title = L.T("Seleccionar rclone.conf personalizado"), Filter = L.T("Configuración rclone (*.conf)|*.conf|Todos los archivos|*.*") };
                    if (configPicker.ShowDialog(this) != true) { result = null; break; }
                    await rclone.ImportConfigAsync(configPicker.FileName);
                    result = new { locked = rclone.Locked, content = rclone.Locked ? null : await rclone.ReadConfigAsync() }; break;
                case "rclone.config-read": result = new { content = await rclone.ReadConfigAsync(), path = rclone.ConfigFilePath }; break;
                case "rclone.config-save": await rclone.SaveConfigAsync(Get("content")); result = rclone.Info(); break;
                case "rclone.info": result = rclone.Info(); break;
                case "rclone.protect": await rclone.ProtectAsync(Get("password")); result = rclone.Info(); break;
                case "rclone.unlock": await rclone.UnlockAsync(Get("password")); result = rclone.Info(); break;
                case "rclone.lock": await StopPlayersAsync(); await rclone.LockAsync(); result = rclone.Info(); break;
                case "rclone.play":
                    if (players.Count >= 2) throw new IOException(L.T("Cierra un reproductor antes de abrir otro."));
                    var media = new MediaWindow(runtime, rclone, Get("path")) {Owner = this};
                    players.Add(media); media.Closed += (_, _) => players.Remove(media); media.Show(); result = true; break;
                case "rclone.advanced": ShowAdvanced(); result = true; break;
                case "rclone.retry":
                    if (automation is not null && automation.State.Queue.Any(item => item.Id == Get("id"))) { automation.Retry(Get("id")); result = automation.State.Queue.Last().Id; }
                    else result = rclone.Retry(Get("id")); break;
                case "rclone.configure":
                    await rclone.ConfigureAsync(parameters.Deserialize<RcloneConnection>(new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? throw new ArgumentException(L.T("Conexión no válida."))); result = rclone.Info(); break;
                case "rclone.browse": result = await rclone.BrowseAsync(Get("path")); break;
                case "rclone.size": result = await rclone.SizeAsync(Get("path")); break;
                case "rclone.link": result = await rclone.LinkAsync(Get("path")); break;
                case "rclone.mkdir": await rclone.CreateFolderAsync(Get("path")); result = true; break;
                case "rclone.rename": await rclone.RenameAsync(Get("source"), Get("target")); result = true; break;
                case "rclone.trash": await rclone.TrashAsync(Get("path"), parameters.GetProperty("directory").GetBoolean()); result = true; break;
                case "rclone.jobs":
                    var ownedProcesses = automation?.State.Queue.Where(item => item.ProcessId is not null).Select(item => item.ProcessId!).ToHashSet() ?? [];
                    var direct = rclone.List().Where(item => !ownedProcesses.Contains(JsonSerializer.SerializeToElement(item).GetProperty("id").GetString()!));
                    result = direct.Concat(automation?.State.Queue.Select(item => (object)new { id = item.Id, label = item.Name, mount = false, status = item.State, log = item.Log }) ?? []).ToArray(); break;
                case "rclone.mount": result = rclone.Mount(Get("drive")); break;
                case "rclone.stop":
                    if (automation is not null && automation.State.Queue.Any(item => item.Id == Get("id"))) await automation.CancelAsync(Get("id"));
                    else await rclone.StopAsync(Get("id")); result = true; break;
                case "rclone.winfsp": await runtime.InstallAsync("winfsp-2.1.25156.msi", true); result = rclone.Info(); break;
                case "rclone.local-folder":
                    var folder = new Microsoft.Win32.OpenFolderDialog { Title = L.T("Carpeta local para Teldrive") };
                    if (folder.ShowDialog(this) == true) localFolder = Path.GetFullPath(folder.FolderName);
                    result = localFolder is null ? null : LocalFiles(""); break;
                case "rclone.local-browse": result = LocalFiles(Get("path")); break;
                case "rclone.upload-local":
                    var localSource = LocalPath(Get("path"));
                    result = QueueUpload(localSource, Get("destination")); break;
                case "rclone.upload-folder":
                    var uploadFolder = new Microsoft.Win32.OpenFolderDialog { Title = L.T("Carpeta para subir a Teldrive") };
                    result = uploadFolder.ShowDialog(this) == true ? QueueUpload(uploadFolder.FolderName, Get("path")) : null; break;
                case "rclone.upload":
                    var choose = new Microsoft.Win32.OpenFileDialog { Title = L.T("Archivo para subir a Teldrive"), Multiselect = false };
                    result = choose.ShowDialog(this) == true ? QueueUpload(choose.FileName, Get("path")) : null; break;
                case "rclone.download":
                    var destination = new Microsoft.Win32.OpenFolderDialog { Title = L.T("Guardar archivos de Teldrive") };
                    result = destination.ShowDialog(this) == true ? automation!.Enqueue(new RcloneTaskDefinition { Name = QueueLabel(L.T("Descargar ") + Get("path")), Direction = "download", Source = Get("path"), Destination = destination.FolderName }).Id : null; break;
                default: throw new ArgumentException(L.T("Operación no admitida."));
            }
            if (!closing && Browser.CoreWebView2 is not null) Browser.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(new { id, result }));
        } catch (Exception error) { if (id is not null && !closing && Browser.CoreWebView2 is not null) Browser.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(new { id, error = error.Message })); }
    }
    private string QueueUpload(string source, string destination)
    {
        var folder = Directory.Exists(source);
        return automation!.Enqueue(new RcloneTaskDefinition { Name = QueueLabel(L.T("Subir ") + Path.GetFileName(source)), Source = source, Destination = folder ? destination.TrimEnd('/') + "/" + Path.GetFileName(source) : destination }).Id;
    }
    private static string QueueLabel(string value) => value.Length > 160 ? value[..157] + "…" : value;
    private async Task RequireOwnerAsync()
    {
        var cookies = await Browser.CoreWebView2.CookieManager.GetCookiesAsync(runtime.ServerUri!.AbsoluteUri);
        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(10) };
        using var request = new HttpRequestMessage(HttpMethod.Get, new Uri(runtime.ServerUri, "/api/v1/me"));
        request.Headers.TryAddWithoutValidation(L.T("Cookie"), string.Join("; ", cookies.Select(cookie => cookie.Name + "=" + cookie.Value)));
        using var response = await http.SendAsync(request);
        if (!response.IsSuccessStatusCode) throw new IOException(L.T("Inicia sesión de nuevo para usar las integraciones de Windows."));
        using var user = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        if (user.RootElement.GetProperty("role").GetString() != "owner") throw new IOException(L.T("Solo el propietario del servidor puede gestionar sus integraciones locales."));
    }
    private string LocalPath(string relative)
    {
        if (localFolder is null || Path.IsPathRooted(relative)) throw new ArgumentException(L.T("Elige primero una carpeta local."));
        var target = Path.GetFullPath(Path.Combine(localFolder, relative));
        if (target != localFolder && !target.StartsWith(localFolder.TrimEnd(Path.DirectorySeparatorChar) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new ArgumentException(L.T("La ruta está fuera de la carpeta seleccionada."));
        var current = target;
        while (current != localFolder && current.Length > localFolder.Length) { if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0) throw new IOException(L.T("Abre la carpeta real desde el selector de Windows.")); current = Path.GetDirectoryName(current)!; }
        return target;
    }
    private object LocalFiles(string relative)
    {
        var folder = new DirectoryInfo(LocalPath(relative));
        return new { path = relative, name = folder.Name, items = folder.EnumerateFileSystemInfos().Where(item => (item.Attributes & FileAttributes.ReparsePoint) == 0).OrderByDescending(item => item is DirectoryInfo).ThenBy(item => item.Name).Take(2000).Select(item => new { name = item.Name, path = Path.GetRelativePath(localFolder!, item.FullName), directory = item is DirectoryInfo, size = item is FileInfo file ? file.Length : 0, modified = item.LastWriteTimeUtc }).ToArray() };
    }
}
