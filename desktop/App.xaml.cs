using System.Windows;

namespace Teldrive.Desktop;

public partial class App : Application
{
    private Mutex? instance;
    protected override void OnStartup(StartupEventArgs e)
    {
        base.OnStartup(e);
        instance = new Mutex(true, "Local\\TeldriveDesktopV2", out var first);
        if (!first) { MessageBox.Show("Teldrive v2 ya está abierto. Busca su ventana en la barra de tareas.", "Teldrive Desktop v2"); Shutdown(); return; }
        DispatcherUnhandledException += (_, error) => { MessageBox.Show(error.Exception.Message, "Teldrive Desktop v2"); error.Handled = true; };
        new MainWindow(e.Args).Show();
    }
    protected override void OnExit(ExitEventArgs e) { instance?.Dispose(); base.OnExit(e); }
}
