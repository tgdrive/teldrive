using System.Windows;

namespace Teldrive.Desktop;

public partial class App : Application
{
    private Mutex? instance;
    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);
        var rootIndex = Array.IndexOf(e.Args, "--data-root");
        var mutexName = "Local\\TeldriveDesktopV2";
        if (rootIndex >= 0 && rootIndex + 1 < e.Args.Length) {
            var root = System.IO.Path.GetFullPath(e.Args[rootIndex + 1]).TrimEnd(System.IO.Path.DirectorySeparatorChar).ToUpperInvariant();
            mutexName += "-" + Convert.ToHexString(System.Security.Cryptography.SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(root)))[..16];
        }
        instance = new Mutex(true, mutexName, out var first);
        if (!first) { MessageBox.Show("Teldrive v2 ya está abierto. Busca su ventana en la barra de tareas.", "Teldrive Desktop v2"); Shutdown(); return; }
        DispatcherUnhandledException += (_, error) => { MessageBox.Show(error.Exception.Message, "Teldrive Desktop v2"); error.Handled = true; };
        new MainWindow(e.Args).Show();
    }
    protected override void OnExit(ExitEventArgs e) { instance?.Dispose(); base.OnExit(e); }
}
