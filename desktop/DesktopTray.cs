using Drawing = System.Drawing;
using Forms = System.Windows.Forms;

namespace Teldrive.Desktop;

internal sealed class DesktopTray : IDisposable
{
    private readonly Forms.NotifyIcon icon;
    private readonly Drawing.Icon cloud;
    private readonly Forms.ContextMenuStrip menu;
    private readonly Forms.ToolStripMenuItem pause;
    private bool always;
    private bool hidden;
    private bool temporary;
    private bool disposed;
    private readonly Queue<Action> pending = new();
    private readonly System.Windows.Threading.Dispatcher dispatcher = System.Windows.Application.Current.Dispatcher;
    private readonly Forms.Timer notificationTimer = new() { Interval = 10000 };
    public DesktopTray(Action restore, Action advanced, Action togglePause, Action exit)
    {
        using var image = System.Windows.Application.GetResourceStream(new Uri("pack://application:,,,/Assets/Teldrive.ico")).Stream;
        cloud = new Drawing.Icon(image, 32, 32);
        menu = new Forms.ContextMenuStrip();
        menu.Closed += (_, _) => { var actions = pending.ToArray(); pending.Clear(); foreach (var action in actions) Dispatch(action); };
        menu.Items.Add(L.T("Abrir Teldrive"), null, (_, _) => Dispatch(restore));
        menu.Items.Add(L.T("Opciones adicionales de rclone"), null, (_, _) => Dispatch(advanced));
        pause = new Forms.ToolStripMenuItem(L.T("Pausar cola"), null, (_, _) => Dispatch(togglePause)); menu.Items.Add(pause);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add(L.T("Salir y detener servicios"), null, (_, _) => Dispatch(exit));
        icon = new Forms.NotifyIcon { Icon = cloud, Text = L.T("Teldrive Desktop v2"), ContextMenuStrip = menu };
        icon.MouseClick += (_, e) => { if (e.Button == Forms.MouseButtons.Left) Dispatch(restore); };
        icon.BalloonTipClicked += (_, _) => Dispatch(() => { restore(); advanced(); });
        notificationTimer.Tick += (_, _) => { notificationTimer.Stop(); temporary = false; icon.Visible = always || hidden; };
    }
    // WebView2/WPF actions must run after the WinForms native menu loop closes.
    private void Dispatch(Action action)
    {
        if (disposed || dispatcher.HasShutdownStarted) return;
        if (menu.Visible) { pending.Enqueue(action); return; }
        _ = dispatcher.BeginInvoke(System.Windows.Threading.DispatcherPriority.ContextIdle, new Action(() => {
            if (disposed) return;
            if (menu.Visible) { pending.Enqueue(action); return; }
            try { action(); }
            catch (Exception error) { System.Windows.MessageBox.Show(error.Message, L.T("Teldrive Desktop v2")); }
        }));
    }
    internal void InvokeMenuForSmoke(int index) => ((Forms.ToolStripMenuItem)menu.Items[index]).PerformClick();
    public bool Visible => icon.Visible;
    public void Apply(DesktopPreferences preferences, bool hidden) {
        always = preferences.AlwaysShowInTray; this.hidden = hidden; icon.Visible = hidden || always || temporary;
        menu.Items[0].Text = L.T("Abrir Teldrive"); menu.Items[1].Text = L.T("Opciones adicionales de rclone"); menu.Items[4].Text = L.T("Salir y detener servicios");
    }
    public void HideWindowIcon() { hidden = true; icon.Visible = true; }
    public void Restored() { hidden = false; icon.Visible = always || temporary; }
    public void QueueState(bool paused, int pending)
    {
        pause.Text = paused ? L.T("Reanudar cola") : L.T("Pausar cola");
        var tooltip = L.T("Teldrive \u00B7 {0} pendientes \u00B7 {1}", pending, (paused ? "cola pausada" : "activo"));
        icon.Text = tooltip.Length > 63 ? tooltip[..63] : tooltip;
    }
    public void Notify(RcloneQueueItem item)
    {
        // Windows needs a visible icon for balloons. Remove a temporary one afterwards.
        if (!icon.Visible) { temporary = true; icon.Visible = true; }
        if (temporary) { notificationTimer.Stop(); notificationTimer.Start(); }
        icon.ShowBalloonTip(5000, L.T("Teldrive · ") + item.StateDisplay, item.Name.Length > 180 ? item.Name[..180] : item.Name, item.State == "completed" ? Forms.ToolTipIcon.Info : Forms.ToolTipIcon.Warning);
    }
    public void Dispose() { if (disposed) return; disposed = true; pending.Clear(); notificationTimer.Stop(); notificationTimer.Dispose(); icon.Visible = false; icon.Dispose(); menu.Dispose(); cloud.Dispose(); }
}
