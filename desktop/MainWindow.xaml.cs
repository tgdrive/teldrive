using System.Diagnostics;
using System.IO;
using System.Net.Http;
using System.Text.Json;
using System.Windows;
using Microsoft.Web.WebView2.Core;

namespace Teldrive.Desktop;

public partial class MainWindow : Window
{
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
    public MainWindow(string[] args)
    {
        InitializeComponent(); arguments = args;
        if (args.Contains("--smoke-test") || args.Contains("--smoke-rclone") || args.Contains("--smoke-server")) { Opacity = 0; ShowInTaskbar = false; ShowActivated = false; }
        var rootIndex = Array.IndexOf(args, "--data-root");
        var portable = args.Contains("--portable") || File.Exists(Path.Combine(AppContext.BaseDirectory, "Teldrive.portable"));
        runtime = new RuntimeSession(rootIndex >= 0 && rootIndex + 1 < args.Length ? args[rootIndex + 1] : portable ? Path.Combine(AppContext.BaseDirectory, "TeldriveData") : null);
        rclone = new RcloneRuntime(runtime);
        runtime.Activity += line => Dispatcher.BeginInvoke(() => { Logs.AppendText(line + "\n"); Logs.ScrollToEnd(); });
        Loaded += async (_, _) => await Guard(async () => {
            await runtime.PrepareAsync();
            if (args.Contains("--smoke-rclone")) { await RcloneSmokeTest.RunAsync(runtime); Close(); return; }
            if (args.Contains("--smoke-server")) {
                if (runtime.State is not null || File.Exists(runtime.ConfigPath)) throw new IOException("La prueba del servidor requiere una carpeta nueva.");
                var probe = new System.Net.Sockets.TcpListener(System.Net.IPAddress.Loopback, 0); probe.Start(); var port = ((System.Net.IPEndPoint)probe.LocalEndpoint).Port; probe.Stop();
                await runtime.ConfigureAsync(port, true, "", ""); await ShowDrive();
                var rendered = false;
                for (var attempt = 0; attempt < 60; attempt++) {
                    var value = await Browser.CoreWebView2.ExecuteScriptAsync("JSON.stringify({title:document.title, text:document.body.innerText})");
                    var json = JsonSerializer.Deserialize<string>(value);
                    if (json is not null && json.Contains("Iniciar sesión", StringComparison.OrdinalIgnoreCase)) { rendered = true; break; }
                    await Task.Delay(500);
                }
                if (!rendered) throw new IOException("El servidor respondió, pero la pantalla de inicio de sesión no se mostró.");
                var originalConfig = await File.ReadAllTextAsync(runtime.ConfigPath);
                var rejected = false;
                try { await runtime.SaveConfigAsync(originalConfig.Replace($"127.0.0.1:{port}", $"203.0.113.2:{port}")); }
                catch (IOException) { rejected = true; }
                if (!rejected || await File.ReadAllTextAsync(runtime.ConfigPath) != originalConfig || runtime.ServerUri!.Port != port) throw new IOException("Una dirección no local modificó la configuración válida.");
                await runtime.StopAsync();
                await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "server-smoke-test.json"), JsonSerializer.Serialize(new {success=true, postgresql=true, migrations=true, loginUi=true, rejectedNonLocalAddress=rejected, stopped=!runtime.Running, port}));
                Close(); return;
            }
            RefreshConfiguration();
            Status.Text = "Listo para configurar tu unidad";
            if (runtime.State is not null && File.Exists(runtime.ConfigPath)) { await runtime.StartAsync(); await ShowDrive(); }
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
            closing = true;
            try {
                if (currentOperation is not null) { try { await currentOperation; } catch { /* Guard already reports the operation failure; cleanup must still run. */ } }
                await StopPlayersAsync(); await rclone.DisposeAsync(); await runtime.DisposeAsync(); Browser.Dispose(); closed = true; Close();
            }
            catch (Exception error) { closing = false; MessageBox.Show(error.Message, "No se pudieron detener los servicios"); }
        };
    }
    private async Task Guard(Func<Task> operation)
    {
        if (busy) return;
        busy = true; StartButton.IsEnabled = false; StopButton.IsEnabled = false;
        try { currentOperation = operation(); await currentOperation; }
        catch (Exception error) {
            Status.Text = error.Message;
            if (arguments.Contains("--smoke-test") || arguments.Contains("--smoke-rclone") || arguments.Contains("--smoke-server")) { await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "smoke-test.json"), JsonSerializer.Serialize(new {success = false, error = error.ToString()})); Close(); }
            else MessageBox.Show(error.Message, "Teldrive Desktop v2", MessageBoxButton.OK, MessageBoxImage.Information);
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
        try { _ = CoreWebView2Environment.GetAvailableBrowserVersionString(); }
        catch (WebView2RuntimeNotFoundException) {
            Status.Text = "Preparando WebView2 integrado…";
            await runtime.InstallAsync("WebView2RuntimeInstaller.exe", false, "/silent", "/install");
        }
        var environment = await CoreWebView2Environment.CreateAsync(userDataFolder: Path.Combine(runtime.DataRoot, "webview"));
        await Browser.EnsureCoreWebView2Async(environment);
        if (Browser.CoreWebView2 is null) throw new IOException("No se pudo inicializar WebView2.");
        Browser.CoreWebView2.Settings.IsStatusBarEnabled = false;
        Browser.CoreWebView2.Settings.AreDevToolsEnabled = false;
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
        if (!runtime.Running) { SetupView.Visibility = Visibility.Visible; Browser.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; return; }
        await InitializeBrowser();
        SetupView.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; Browser.Visibility = Visibility.Visible;
        Browser.Source = new Uri(runtime.ServerUri!, path);
        Status.Text = $"Conectado · {runtime.ServerUri} · Datos guardados en tu equipo";
    }
    private async void StartClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.StartAsync(); await ShowDrive(); });
    private async void StopClick(object sender, RoutedEventArgs e) => await Guard(async () => { await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.StopAsync(); Browser.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Visible; Status.Text = "Servicios detenidos"; });
    private void SetupClick(object sender, RoutedEventArgs e) { RefreshConfiguration(); Browser.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Visible; }
    private void LogsClick(object sender, RoutedEventArgs e) { Browser.Visibility = Visibility.Collapsed; SetupView.Visibility = Visibility.Collapsed; LogsView.Visibility = Visibility.Visible; }
    private async void DriveClick(object sender, RoutedEventArgs e) => await Guard(() => ShowDrive());
    private async void RcloneClick(object sender, RoutedEventArgs e) => await Guard(() => ShowDrive("/settings/rclone"));
    private async void ConfigureClick(object sender, RoutedEventArgs e) => await Guard(async () => {
        if (!int.TryParse(HttpPort.Text, out var port)) throw new ArgumentException("Introduce un puerto válido.");
        Status.Text = "Preparando la base de datos y el servidor…";
        await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.ConfigureAsync(port, LocalDatabase.IsChecked == true, DatabaseUrl.Text.Trim(), AllowedUser.Text); RefreshConfiguration(); await ShowDrive();
    });
    private async void SaveConfigClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.SaveConfigAsync(ConfigEditor.Text); await StopPlayersAsync(); await rclone.StopAllAsync(); await runtime.StopAsync(); await runtime.StartAsync(); await ShowDrive(); });
    private async void ValidateClick(object sender, RoutedEventArgs e) => await Guard(async () => { await runtime.ValidateConfigAsync(ConfigEditor.Text); Status.Text = "Configuración válida"; });
    private async void WinFspClick(object sender, RoutedEventArgs e) => await Guard(() => runtime.InstallAsync("winfsp-2.1.25156.msi", true));
    private async void RuntimeClick(object sender, RoutedEventArgs e) => await Guard(() => runtime.InstallAsync("VC_redist.x64.exe", false, "/install", "/passive", "/norestart"));
    private void DataClick(object sender, RoutedEventArgs e) => OpenExternal(runtime.DataRoot);
    private static void OpenExternal(string target) => Process.Start(new ProcessStartInfo(target) { UseShellExecute = true });
    private async void ReceiveNativeMessage(object? sender, CoreWebView2WebMessageReceivedEventArgs e)
    {
        if (closing || !runtime.Running || runtime.ServerUri is null || !Uri.TryCreate(e.Source, UriKind.Absolute, out var source)
            || source.GetLeftPart(UriPartial.Authority) != runtime.ServerUri.GetLeftPart(UriPartial.Authority)
            || source.AbsolutePath is not ("/settings/rclone" or "/settings/server")) return;
        string? id = null;
        try {
            if (e.WebMessageAsJson.Length > 1024 * 1024) throw new IOException("La solicitud es demasiado grande.");
            using var message = JsonDocument.Parse(e.WebMessageAsJson);
            id = message.RootElement.GetProperty("id").GetString();
            if (id is null || !Guid.TryParse(id, out _)) return;
            await RequireOwnerAsync();
            var method = message.RootElement.GetProperty("method").GetString();
            var parameters = message.RootElement.GetProperty("params");
            string Get(string key) => parameters.GetProperty(key).GetString() ?? "";
            object? result;
            switch (method) {
                case "rclone.info": result = rclone.Info(); break;
                case "rclone.protect": await rclone.ProtectAsync(Get("password")); result = rclone.Info(); break;
                case "rclone.unlock": await rclone.UnlockAsync(Get("password")); result = rclone.Info(); break;
                case "rclone.lock": await StopPlayersAsync(); await rclone.LockAsync(); result = rclone.Info(); break;
                case "rclone.play":
                    if (players.Count >= 2) throw new IOException("Cierra un reproductor antes de abrir otro.");
                    var media = new MediaWindow(runtime, rclone, Get("path")) {Owner = this};
                    players.Add(media); media.Closed += (_, _) => players.Remove(media); media.Show(); result = true; break;
                case "rclone.retry": result = rclone.Retry(Get("id")); break;
                case "rclone.configure":
                    await rclone.ConfigureAsync(parameters.Deserialize<RcloneConnection>(new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? throw new ArgumentException("Conexión no válida.")); result = rclone.Info(); break;
                case "rclone.browse": result = await rclone.BrowseAsync(Get("path")); break;
                case "rclone.size": result = await rclone.SizeAsync(Get("path")); break;
                case "rclone.link": result = await rclone.LinkAsync(Get("path")); break;
                case "rclone.mkdir": await rclone.CreateFolderAsync(Get("path")); result = true; break;
                case "rclone.rename": await rclone.RenameAsync(Get("source"), Get("target")); result = true; break;
                case "rclone.trash": await rclone.TrashAsync(Get("path"), parameters.GetProperty("directory").GetBoolean()); result = true; break;
                case "rclone.jobs": result = rclone.List(); break;
                case "rclone.mount": result = rclone.Mount(Get("drive")); break;
                case "rclone.stop": await rclone.StopAsync(Get("id")); result = true; break;
                case "rclone.winfsp": await runtime.InstallAsync("winfsp-2.1.25156.msi", true); result = rclone.Info(); break;
                case "rclone.local-folder":
                    var folder = new Microsoft.Win32.OpenFolderDialog { Title = "Carpeta local para Teldrive" };
                    if (folder.ShowDialog(this) == true) localFolder = Path.GetFullPath(folder.FolderName);
                    result = localFolder is null ? null : LocalFiles(""); break;
                case "rclone.local-browse": result = LocalFiles(Get("path")); break;
                case "rclone.upload-local":
                    var localSource = LocalPath(Get("path"));
                    result = Directory.Exists(localSource) ? rclone.UploadFolder(localSource, Get("destination")) : rclone.Upload(localSource, Get("destination")); break;
                case "rclone.upload-folder":
                    var uploadFolder = new Microsoft.Win32.OpenFolderDialog { Title = "Carpeta para subir a Teldrive" };
                    result = uploadFolder.ShowDialog(this) == true ? rclone.UploadFolder(uploadFolder.FolderName, Get("path")) : null; break;
                case "rclone.upload":
                    var choose = new Microsoft.Win32.OpenFileDialog { Title = "Archivo para subir a Teldrive", Multiselect = false };
                    result = choose.ShowDialog(this) == true ? rclone.Upload(choose.FileName, Get("path")) : null; break;
                case "rclone.download":
                    var destination = new Microsoft.Win32.OpenFolderDialog { Title = "Guardar archivos de Teldrive" };
                    result = destination.ShowDialog(this) == true ? rclone.Download(Get("path"), destination.FolderName) : null; break;
                default: throw new ArgumentException("Operación no admitida.");
            }
            if (!closing && Browser.CoreWebView2 is not null) Browser.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(new { id, result }));
        } catch (Exception error) { if (id is not null && !closing && Browser.CoreWebView2 is not null) Browser.CoreWebView2.PostWebMessageAsJson(JsonSerializer.Serialize(new { id, error = error.Message })); }
    }
    private async Task RequireOwnerAsync()
    {
        var cookies = await Browser.CoreWebView2.CookieManager.GetCookiesAsync(runtime.ServerUri!.AbsoluteUri);
        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(10) };
        using var request = new HttpRequestMessage(HttpMethod.Get, new Uri(runtime.ServerUri, "/api/v1/me"));
        request.Headers.TryAddWithoutValidation("Cookie", string.Join("; ", cookies.Select(cookie => cookie.Name + "=" + cookie.Value)));
        using var response = await http.SendAsync(request);
        if (!response.IsSuccessStatusCode) throw new IOException("Inicia sesión de nuevo para usar las integraciones de Windows.");
        using var user = JsonDocument.Parse(await response.Content.ReadAsStringAsync());
        if (user.RootElement.GetProperty("role").GetString() != "owner") throw new IOException("Solo el propietario del servidor puede gestionar sus integraciones locales.");
    }
    private string LocalPath(string relative)
    {
        if (localFolder is null || Path.IsPathRooted(relative)) throw new ArgumentException("Elige primero una carpeta local.");
        var target = Path.GetFullPath(Path.Combine(localFolder, relative));
        if (target != localFolder && !target.StartsWith(localFolder.TrimEnd(Path.DirectorySeparatorChar) + Path.DirectorySeparatorChar, StringComparison.OrdinalIgnoreCase)) throw new ArgumentException("La ruta está fuera de la carpeta seleccionada.");
        var current = target;
        while (current != localFolder && current.Length > localFolder.Length) { if ((File.GetAttributes(current) & FileAttributes.ReparsePoint) != 0) throw new IOException("Abre la carpeta real desde el selector de Windows."); current = Path.GetDirectoryName(current)!; }
        return target;
    }
    private object LocalFiles(string relative)
    {
        var folder = new DirectoryInfo(LocalPath(relative));
        return new { path = relative, name = folder.Name, items = folder.EnumerateFileSystemInfos().Where(item => (item.Attributes & FileAttributes.ReparsePoint) == 0).OrderByDescending(item => item is DirectoryInfo).ThenBy(item => item.Name).Take(2000).Select(item => new { name = item.Name, path = Path.GetRelativePath(localFolder!, item.FullName), directory = item is DirectoryInfo, size = item is FileInfo file ? file.Length : 0, modified = item.LastWriteTimeUtc }).ToArray() };
    }
}
