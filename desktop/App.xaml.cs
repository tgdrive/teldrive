using System.Windows;

namespace Teldrive.Desktop;

public partial class App : Application
{
    private Mutex? instance;
    private EventWaitHandle? showWindow;
    private RegisteredWaitHandle? showRegistration;
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
        if (!first) {
            try { using var signal = EventWaitHandle.OpenExisting(mutexName + "-Show"); signal.Set(); }
            catch (WaitHandleCannotBeOpenedException) { MessageBox.Show(L.T("Teldrive v2 ya está iniciándose. Busca su icono en la bandeja del sistema."), L.T("Teldrive Desktop v2")); }
            Shutdown(); return;
        }
        DispatcherUnhandledException += (_, error) => { MessageBox.Show(error.Exception.Message, L.T("Teldrive Desktop v2")); error.Handled = true; };
        var window = new MainWindow(e.Args);
        MainWindow = window;
        showWindow = new EventWaitHandle(false, EventResetMode.AutoReset, mutexName + "-Show");
        showRegistration = ThreadPool.RegisterWaitForSingleObject(showWindow, (_, _) => Dispatcher.BeginInvoke(window.RestoreFromTray), null, Timeout.Infinite, false);
        window.Show();
    }
    protected override void OnExit(ExitEventArgs e) { showRegistration?.Unregister(null); showWindow?.Dispose(); instance?.Dispose(); base.OnExit(e); }
}
