using System.IO;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Media;
using System.Windows.Media.Imaging;

namespace Teldrive.Desktop;

public partial class MainWindow
{
    internal async Task VerifyNavigationAsync()
    {
        await ShowDrive(); await Task.Delay(1200);
        Logs.AppendText("Prueba de actividad: registro visible sin una capa del navegador.\n");
        LogsClick(this, new RoutedEventArgs()); await Task.Delay(200); UpdateLayout();
        if (!LogsView.IsVisible || Browser.IsVisible || SetupView.IsVisible || AdvancedView.IsVisible) throw new IOException("Actividad no ocultó las demás vistas.");
        var image = new RenderTargetBitmap((int)LogsView.ActualWidth, (int)LogsView.ActualHeight, 96, 96, PixelFormats.Pbgra32); image.Render(LogsView);
        var pixels = new byte[4]; image.CopyPixels(new Int32Rect((int)LogsView.ActualWidth / 2, (int)LogsView.ActualHeight / 2, 1, 1), pixels, 4, 0);
        if (pixels[0] > 100 || pixels[1] > 100 || pixels[2] > 100) throw new IOException("Actividad muestra una capa clara en el centro del registro.");
        var encoder = new PngBitmapEncoder(); encoder.Frames.Add(BitmapFrame.Create(image)); using (var output = File.Create(Path.Combine(runtime.DataRoot, "activity.png"))) encoder.Save(output);
        // Exercise the actual header button after initialization, as a user would.
        busy = false; DriveButton.RaiseEvent(new RoutedEventArgs(Button.ClickEvent));
        for (var attempt=0;attempt<40&&!Browser.IsVisible;attempt++) await Task.Delay(50);
        if (!Browser.IsVisible || LogsView.IsVisible) throw new IOException("El botón Teldrive no recuperó el Drive.");
        await Task.Delay(100); busy = true;
        ShowAdvanced(); if (!AdvancedView.IsVisible || Browser.IsVisible) throw new IOException("Rclone no ocultó el navegador.");
        SetupClick(this,new RoutedEventArgs()); if (!SetupView.IsVisible || AdvancedView.IsVisible) throw new IOException("Configuración no recuperó su vista.");
        await ShowDrive();
    }
}
