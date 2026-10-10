using System.Runtime.InteropServices;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Interop;
using System.Windows.Media;

namespace Teldrive.Desktop;

internal sealed class MediaWindow : Window
{
    private readonly MediaSession session;
    private bool disposed;
    private readonly TaskCompletionSource stopped = new();
    public async Task StopAsync() { Close(); await stopped.Task; }
    public MediaWindow(RuntimeSession runtime, RcloneRuntime rclone, string path)
    {
        session = new MediaSession(runtime);
        Title = "Teldrive · " + System.IO.Path.GetFileName(path); Width = 1000; Height = 700; MinWidth = 500; MinHeight = 350; Background = Brushes.Black;
        var panel = new DockPanel();
        var note = new TextBlock {Text = "Controles de mpv: espacio para pausar · flechas para avanzar o retroceder · doble clic para pantalla completa", Foreground = Brushes.White, Margin = new Thickness(12), TextWrapping = TextWrapping.Wrap};
        DockPanel.SetDock(note, Dock.Bottom); panel.Children.Add(note);
        var surface = new PlayerSurface(); panel.Children.Add(surface); Content = panel;
        Loaded += async (_, _) => {
            try { await session.StartAsync(rclone, path, surface.Window); }
            catch (Exception error) { if (!disposed) MessageBox.Show(this, error.Message, "Reproducción multimedia"); }
            finally { if (!disposed) Close(); }
        };
        Closing += async (_, e) => { if (disposed) return; e.Cancel = true; disposed = true; try { await session.DisposeAsync(); Close(); } finally { stopped.TrySetResult(); } };
    }
    private sealed class PlayerSurface : HwndHost
    {
        public nint Window { get; private set; }
        protected override HandleRef BuildWindowCore(HandleRef parent)
        {
            Window = CreateWindowEx(0, "static", "", 0x40000000 | 0x10000000 | 0x02000000, 0, 0, 1, 1, parent.Handle, 0, 0, 0);
            if (Window == 0) throw new InvalidOperationException("No se pudo crear la superficie de vídeo.");
            return new HandleRef(this, Window);
        }
        protected override void DestroyWindowCore(HandleRef hwnd) { _ = DestroyWindow(hwnd.Handle); Window = 0; }
        [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern nint CreateWindowEx(uint styleEx, string className, string title, uint style, int x, int y, int width, int height, nint parent, nint menu, nint instance, nint param);
        [DllImport("user32.dll")] private static extern bool DestroyWindow(nint window);
    }
}
