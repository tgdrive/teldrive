using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;
using System.Windows.Interop;
using System.Windows.Media;
using System.Windows.Threading;

namespace Teldrive.Desktop;

internal sealed class MediaWindow : Window
{
    private readonly MediaSession session;
    private bool disposed, fullScreen, updating;
    private readonly WindowFullscreen fullscreen;
    private readonly Button fullScreenButton = new() { Content = "Pantalla completa (F11)", Margin = new Thickness(4) };
    private readonly TextBox statistics = new() { IsReadOnly = true, TextWrapping = TextWrapping.Wrap, Background = Brushes.Black, Foreground = Brushes.White, BorderThickness = new Thickness(0), FontFamily = new FontFamily("Consolas"), FontSize = 12, Padding = new Thickness(12), VerticalScrollBarVisibility = ScrollBarVisibility.Auto, Width = 400, Visibility = Visibility.Collapsed };
    private readonly DispatcherTimer timer = new() { Interval = TimeSpan.FromSeconds(1) };
    private readonly CancellationTokenSource lifetime = new();
    private Task<NativeStreamStatistics?>? transport;
    private readonly TaskCompletionSource stopped = new();
    internal MediaTelemetry TelemetryForSmoke => session.Telemetry;
    internal bool FullscreenForSmoke => fullScreen;
    public async Task StopAsync() { Close(); await stopped.Task; }
    public MediaWindow(RuntimeSession runtime, RcloneRuntime rclone, string path)
    {
        session = new MediaSession(runtime);
        fullscreen = new WindowFullscreen(this);
        Title = L.T("Teldrive · ") + System.IO.Path.GetFileName(path); Width = 1100; Height = 700; MinWidth = 500; MinHeight = 350; Background = Brushes.Black;
        var panel = new DockPanel();
        var controls = new WrapPanel { Background = new SolidColorBrush(Color.FromRgb(28,28,28)) };
        fullScreenButton.Click += (_, _) => SetFullScreen(!fullScreen);
        controls.Children.Add(fullScreenButton);
        var developer = new Button {Content = "Desarrollador · estadísticas (I)", Margin = new Thickness(4)};
        developer.Click += (_, _) => ToggleStatistics(); controls.Children.Add(developer);
        controls.Children.Add(new TextBlock {Text = "Espacio: pausa · flechas: buscar · Esc: salir de pantalla completa", Foreground = Brushes.White, Margin = new Thickness(8), TextWrapping = TextWrapping.Wrap});
        DockPanel.SetDock(controls, Dock.Bottom); panel.Children.Add(controls);
        DockPanel.SetDock(statistics, Dock.Right); panel.Children.Add(statistics);
        var surface = new PlayerSurface(); panel.Children.Add(surface); Content = panel;
        PreviewKeyDown += (_, e) => { if (e.Key is Key.F11 or Key.F) {SetFullScreen(!fullScreen);e.Handled=true;} else if(e.Key==Key.Escape&&fullScreen){SetFullScreen(false);e.Handled=true;} else if(e.Key==Key.I){ToggleStatistics();e.Handled=true;} };
        session.Telemetry.Control += action => Dispatcher.BeginInvoke(new Action(() => {
            if(disposed)return;
            if(action=="teldrive-statistics")ToggleStatistics();
            else if(action=="teldrive-exit-fullscreen")SetFullScreen(false);
            else SetFullScreen(!fullScreen);
        }));
        timer.Tick += async (_, _) => await UpdateStatisticsAsync();
        Loaded += async (_, _) => {
            transport = CreateTransportAsync(rclone,path);
            try { await session.StartAsync(rclone, path, surface.Window); }
            catch (Exception error) { if (!disposed) MessageBox.Show(this, error.Message, L.T("Reproducción multimedia")); }
            finally { if (!disposed) Close(); }
        };
        Closing += async (_, e) => {
            if (disposed) return; e.Cancel = true; disposed = true; timer.Stop(); lifetime.Cancel();
            try { await session.DisposeAsync(); if(transport is not null)(await transport)?.Dispose(); }
            finally { Close(); stopped.TrySetResult(); }
        };
    }
    private static async Task<NativeStreamStatistics?> CreateTransportAsync(RcloneRuntime rclone,string path) {try{return await NativeStreamStatistics.CreateAsync(rclone,path);}catch{return null;}}
    private void ToggleStatistics() {statistics.Visibility = statistics.Visibility == Visibility.Visible ? Visibility.Collapsed : Visibility.Visible;if(statistics.Visibility==Visibility.Visible){timer.Start();_ = UpdateStatisticsAsync();}else timer.Stop();}
    private async Task UpdateStatisticsAsync()
    {
        if(updating||disposed||statistics.Visibility!=Visibility.Visible)return;updating=true;
        try {
            var text = new StringBuilder("DESARROLLADOR · reproducción\n");
            text.AppendLine("Motor: mpv · IPC " + (session.Telemetry.Connected ? "conectado" : "conectando"));
            foreach(var pair in session.Telemetry.Snapshot()) {
                if(pair.Key=="fullscreen")continue;
                text.AppendLine(pair.Key + ": " + (pair.Value.ValueKind is JsonValueKind.Null or JsonValueKind.Undefined ? "No disponible" : pair.Value.ToString()));
            }
            text.AppendLine();
            var reader = transport is null || !transport.IsCompleted ? null : await transport;
            try {text.AppendLine(reader is null ? "El origen no ofrece estadísticas de Telegram." : await reader.ReadAsync(lifetime.Token));}
            catch(Exception error) when(error is System.Net.Http.HttpRequestException or OperationCanceledException or JsonException){text.AppendLine("Estadísticas de Telegram temporalmente no disponibles.");}
            if(!disposed)statistics.Text=text.ToString();
        } finally {updating=false;}
    }
    internal void SetFullScreen(bool value)
    {
        if(disposed||fullScreen==value)return;
        fullscreen.Set(value);
        fullScreen=value;fullScreenButton.Content=value?"Salir de pantalla completa (Esc)":"Pantalla completa (F11)";
    }
    private sealed class PlayerSurface : HwndHost
    {
        public nint Window { get; private set; }
        protected override HandleRef BuildWindowCore(HandleRef parent)
        {
            Window = CreateWindowEx(0, "static", "", 0x40000000 | 0x10000000 | 0x02000000, 0, 0, 1, 1, parent.Handle, 0, 0, 0);
            if (Window == 0) throw new InvalidOperationException(L.T("No se pudo crear la superficie de vídeo."));
            return new HandleRef(this, Window);
        }
        protected override void DestroyWindowCore(HandleRef hwnd) { _ = DestroyWindow(hwnd.Handle); Window = 0; }
        [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)] private static extern nint CreateWindowEx(uint styleEx, string className, string title, uint style, int x, int y, int width, int height, nint parent, nint menu, nint instance, nint param);
        [DllImport("user32.dll")] private static extern bool DestroyWindow(nint window);
    }
}
