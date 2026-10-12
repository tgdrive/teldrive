using System.IO;
using System.Windows;
using System.Windows.Controls;

namespace Teldrive.Desktop;

public partial class RcloneAdvancedView
{
    private async Task ConfigAction(Func<Task> operation)
    {
        try { if (rclone is null) throw new IOException("Espera a que se preparen los componentes."); await operation(); }
        catch (Exception error) { Feedback.Text = error.Message; }
    }
    private void ShowConfig(string content)
    {
        configContent = content; RawConfigEditor.Text = content;
        var selected = ConfigRemote.SelectedItem as string;
        var sections = RcloneConfigDocument.Sections(content);
        ConfigRemote.ItemsSource = sections;
        ConfigRemote.SelectedItem = sections.Contains(selected) ? selected : sections.FirstOrDefault();
        if (sections.Length == 0) ConfigOptions.ItemsSource = Array.Empty<RcloneConfigOption>();
    }
    private async void LoadConfigFileClick(object sender, RoutedEventArgs e) => await ConfigAction(async () => { ShowConfig(await rclone!.ReadConfigAsync()); Feedback.Text = "Configuración cargada. Los cambios aún no se han guardado."; });
    private async void ImportConfigFileClick(object sender, RoutedEventArgs e) => await ConfigAction(async () => {
        var picker = new Microsoft.Win32.OpenFileDialog { Title = "Seleccionar rclone.conf personalizado", Filter = "Configuración rclone (*.conf)|*.conf|Todos los archivos|*.*" };
        if (picker.ShowDialog(Window.GetWindow(this)) != true) return;
        await rclone!.ImportConfigAsync(picker.FileName);
        if (rclone.Locked) { ShowConfig(""); Feedback.Text = "Archivo cifrado importado. Introduce su contraseña para editarlo y utilizarlo."; }
        else { ShowConfig(await rclone.ReadConfigAsync()); Feedback.Text = "rclone.conf importado. El archivo original se conserva en su ubicación."; }
    });
    private async void UnlockConfigFileClick(object sender, RoutedEventArgs e) => await ConfigAction(async () => { await rclone!.UnlockAsync(ConfigPassword.Password); ConfigPassword.Clear(); ShowConfig(await rclone.ReadConfigAsync()); Feedback.Text = "Configuración desbloqueada."; });
    private async void SaveConfigFileClick(object sender, RoutedEventArgs e) => await ConfigAction(async () => { await rclone!.SaveConfigAsync(ShowRawConfig.IsChecked == true ? RawConfigEditor.Text : configContent); ShowConfig(await rclone.ReadConfigAsync()); Feedback.Text = "rclone.conf guardado y validado. Respaldo anterior: rclone.conf.previous."; });
    private void ExportConfigFileClick(object sender, RoutedEventArgs e)
    {
        try {
            if (rclone is null || !File.Exists(rclone.ConfigFilePath)) throw new IOException("Carga o guarda primero una configuración.");
            var picker = new Microsoft.Win32.SaveFileDialog { Title = "Exportar rclone.conf", FileName = "rclone.conf", Filter = "Configuración rclone (*.conf)|*.conf" };
            if (picker.ShowDialog(Window.GetWindow(this)) == true && !Path.GetFullPath(picker.FileName).Equals(Path.GetFullPath(rclone.ConfigFilePath), StringComparison.OrdinalIgnoreCase)) File.Copy(rclone.ConfigFilePath, picker.FileName, true);
        } catch (Exception error) { Feedback.Text = error.Message; }
    }
    private void ConfigRemoteChanged(object sender, SelectionChangedEventArgs e)
    {
        if (ConfigOptions is null || ConfigRemote.SelectedItem is not string selected) return;
        ConfigOptions.ItemsSource = RcloneConfigDocument.Options(configContent, selected);
    }
    private void ConfigOptionChanged(object sender, SelectionChangedEventArgs e)
    {
        if (ConfigOptionKey is null || ConfigOptions.SelectedItem is not RcloneConfigOption option) return;
        ConfigOptionKey.Text = option.Key; ConfigOptionValue.Text = option.Value; ConfigOptionSecret.Password = option.Value;
        ConfigOptionValue.Visibility = option.Secret ? Visibility.Collapsed : Visibility.Visible;
        ConfigOptionSecret.Visibility = option.Secret ? Visibility.Visible : Visibility.Collapsed;
    }
    private void ApplyConfigOptionClick(object sender, RoutedEventArgs e) => Action(() => {
        var remote = ConfigRemote.SelectedItem as string ?? throw new ArgumentException("Carga y selecciona una conexión.");
        var value = ConfigOptionSecret.Visibility == Visibility.Visible ? ConfigOptionSecret.Password : ConfigOptionValue.Text;
        configContent = RcloneConfigDocument.Set(configContent, remote, ConfigOptionKey.Text.Trim(), value);
        ShowConfig(configContent);
    }, "Opción aplicada al editor; guarda rclone.conf para utilizarla.");
    private void RemoveConfigOptionClick(object sender, RoutedEventArgs e) => Action(() => { configContent = RcloneConfigDocument.Set(configContent, ConfigRemote.SelectedItem as string ?? "", ConfigOptionKey.Text.Trim(), null); ShowConfig(configContent); });
    private void ShowRawConfigChanged(object sender, RoutedEventArgs e)
    {
        if (RawConfigEditor is null) return;
        if (ShowRawConfig.IsChecked != true) { configContent = RawConfigEditor.Text; ShowConfig(configContent); }
        RawConfigEditor.Visibility = ShowRawConfig.IsChecked == true ? Visibility.Visible : Visibility.Collapsed;
    }
}
