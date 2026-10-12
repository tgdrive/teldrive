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
    private readonly Forms.Timer notificationTimer = new() { Interval = 10000 };
    public DesktopTray(Action restore, Action advanced, Action togglePause, Action exit)
    {
        using var image = System.Windows.Application.GetResourceStream(new Uri("pack://application:,,,/Assets/Teldrive.ico")).Stream;
        cloud = new Drawing.Icon(image, 32, 32);
        menu = new Forms.ContextMenuStrip();
        menu.Items.Add("Abrir Teldrive", null, (_, _) => restore());
        menu.Items.Add("Opciones adicionales de rclone", null, (_, _) => advanced());
        pause = new Forms.ToolStripMenuItem("Pausar cola", null, (_, _) => togglePause()); menu.Items.Add(pause);
        menu.Items.Add(new Forms.ToolStripSeparator());
        menu.Items.Add("Salir y detener servicios", null, (_, _) => exit());
        icon = new Forms.NotifyIcon { Icon = cloud, Text = "Teldrive Desktop v2", ContextMenuStrip = menu };
        icon.MouseClick += (_, e) => { if (e.Button == Forms.MouseButtons.Left) restore(); };
        icon.BalloonTipClicked += (_, _) => { restore(); advanced(); };
        notificationTimer.Tick += (_, _) => { notificationTimer.Stop(); temporary = false; icon.Visible = always || hidden; };
    }
    public bool Visible => icon.Visible;
    public void Apply(DesktopPreferences preferences, bool hidden) { always = preferences.AlwaysShowInTray; this.hidden = hidden; icon.Visible = hidden || always || temporary; }
    public void HideWindowIcon() { hidden = true; icon.Visible = true; }
    public void Restored() { hidden = false; icon.Visible = always || temporary; }
    public void QueueState(bool paused, int pending)
    {
        pause.Text = paused ? "Reanudar cola" : "Pausar cola";
        icon.Text = $"Teldrive · {pending} pendientes · {(paused ? "cola pausada" : "activo")}";
    }
    public void Notify(RcloneQueueItem item)
    {
        // Windows needs a visible icon for balloons. Remove a temporary one afterwards.
        if (!icon.Visible) { temporary = true; icon.Visible = true; }
        if (temporary) { notificationTimer.Stop(); notificationTimer.Start(); }
        icon.ShowBalloonTip(5000, "Teldrive · " + item.StateDisplay, item.Name.Length > 180 ? item.Name[..180] : item.Name, item.State == "completed" ? Forms.ToolTipIcon.Info : Forms.ToolTipIcon.Warning);
    }
    public void Dispose() { notificationTimer.Stop(); notificationTimer.Dispose(); icon.Visible = false; icon.Dispose(); menu.Dispose(); cloud.Dispose(); }
}
