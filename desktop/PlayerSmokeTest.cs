using System.IO;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Threading;

namespace Teldrive.Desktop;

internal static class PlayerSmokeTest
{
    public static async Task RunAsync(RuntimeSession runtime)
    {
        var config = Path.Combine(runtime.DataRoot,"rclone.conf");
        if(File.Exists(config))throw new IOException("La prueba requiere datos nuevos.");
        var remote=Path.Combine(runtime.DataRoot,"test-media");Directory.CreateDirectory(remote);
        await File.WriteAllTextAsync(config,$"[teldrive]\ntype = alias\nremote = {remote}\n");
        using(var file=File.Create(Path.Combine(remote,"audio.wav")))using(var writer=new BinaryWriter(file)) {
            const int rate=22050,samples=rate*15;
            writer.Write("RIFF"u8);writer.Write(36+samples*2);writer.Write("WAVEfmt "u8);writer.Write(16);writer.Write((short)1);writer.Write((short)1);writer.Write(rate);writer.Write(rate*2);writer.Write((short)2);writer.Write((short)16);writer.Write("data"u8);writer.Write(samples*2);
            for(var i=0;i<samples;i++)writer.Write((short)0);
        }
        var calls=new List<int>();
        using(var tray=new DesktopTray(()=>calls.Add(0),()=>calls.Add(1),()=>calls.Add(2),()=>calls.Add(4))) {
            foreach(var index in new[]{0,1,2,4})tray.InvokeMenuForSmoke(index);
            Check(calls.Count==0,"El menú ejecutó callbacks dentro del evento nativo.");
            await Dispatcher.Yield(DispatcherPriority.ApplicationIdle);
            Check(calls.SequenceEqual(new[]{0,1,2,4}),"Las opciones de bandeja no se ejecutaron en orden.");
            foreach(var language in L.Languages) {L.Current.Language=language.Id;tray.QueueState(false,int.MaxValue);}
            L.Current.Language="es";
        }
        await using var rclone=new RcloneRuntime(runtime);
        var player=new MediaWindow(runtime,rclone,"/audio.wav") {Opacity=0,ShowInTaskbar=false,ShowActivated=false};
        player.Show();
        try {
            for(var i=0;i<100&&!player.TelemetryForSmoke.Snapshot().ContainsKey("duration");i++)await Task.Delay(100);
            var telemetry=player.TelemetryForSmoke;
            Check(telemetry.Connected&&telemetry.Snapshot().TryGetValue("duration",out var duration)&&duration.GetDouble()>14,"No se recibieron metadatos reales de mpv.");
            await telemetry.SendAsync("set_property","pause",true);
            for(var i=0;i<30&&(!telemetry.Snapshot().TryGetValue("pause",out var paused)||paused.ValueKind!=JsonValueKind.True);i++)await Task.Delay(50);
            Check(telemetry.Snapshot()["pause"].GetBoolean(),"mpv no confirmó la pausa por IPC.");
            var bounds=new Rect(player.Left,player.Top,player.Width,player.Height);var style=player.WindowStyle;
            await telemetry.SendAsync("script-message","teldrive-fullscreen");
            await Task.Delay(200);
            Check(player.FullscreenForSmoke&&player.WindowStyle==WindowStyle.None,"F11/doble clic no activaron pantalla completa en el host.");
            await telemetry.SendAsync("script-message","teldrive-exit-fullscreen");await Task.Delay(200);
            Check(!player.FullscreenForSmoke&&player.WindowStyle==style&&Math.Abs(player.Width-bounds.Width)<2,"Escape no restauró la ventana.");
            await telemetry.SendAsync("script-message","teldrive-statistics");await Task.Delay(300);
            var panel=(DockPanel)player.Content;
            Check(panel.Children.OfType<TextBox>().Any(box=>box.Visibility==Visibility.Visible&&box.Text.Contains("duration")),"No se mostraron las estadísticas del reproductor.");
            await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot,"player-smoke-test.json"),JsonSerializer.Serialize(new {success=true,mpvIpc=true,mediaMetadata=true,pause=true,fullscreen=true,fullscreenRestore=true,developerStatistics=true,trayDeferredCallbacks=true}));
        } finally {await player.StopAsync();}
    }
    private static void Check(bool value,string message){if(!value)throw new IOException(message);}
}
