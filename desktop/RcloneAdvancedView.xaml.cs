using System.Globalization;
using System.IO;
using System.Text.Json;
using System.Windows;
using System.Windows.Controls;
using System.Windows.Input;

namespace Teldrive.Desktop;

public partial class RcloneAdvancedView : UserControl
{
    private RcloneAutomation? automation;
    private string? editingTask;
    private string? editingSchedule;
    private Action<DesktopPreferences>? savePreferences;
    private Action? hideWindow;
    private Action? exit;
    private bool refreshing;
    private RcloneRuntime? rclone;
    private string configContent = "";

    public RcloneAdvancedView() { InitializeComponent(); ScheduleDate.SelectedDate = DateTime.Today.AddDays(1); }
    public void Initialize(RcloneAutomation value, DesktopPreferences preferences, Action<DesktopPreferences> save, Action hide, Action quit, RcloneRuntime runtime)
    {
        rclone = runtime; RcloneConfigPath.Text = runtime.ConfigFilePath; automation = value; savePreferences = save; hideWindow = hide; exit = quit;
        AlwaysTray.IsChecked = preferences.AlwaysShowInTray; MinimizeTray.IsChecked = preferences.MinimizeToTray;
        CloseTray.IsChecked = preferences.CloseToTray; StartTray.IsChecked = preferences.StartInTray;
        StartWindows.IsChecked = preferences.StartWithWindows; NotifyFinished.IsChecked = preferences.NotifyFinished;
        LanguageChoice.ItemsSource = L.Languages; LanguageChoice.SelectedValue = preferences.Language;
        Refresh();
    }
    public void Refresh()
    {
        if (automation is null) return;
        refreshing = true;
        try {
            RefreshGrid(TasksGrid, automation.State.Tasks, item => ((RcloneTaskDefinition)item).Id);
            RefreshGrid(QueueGrid, automation.State.Queue, item => ((RcloneQueueItem)item).Id);
            RefreshGrid(SchedulesGrid, automation.State.Schedules, item => ((RcloneSchedule)item).Id);
            var selected = ScheduleTask.SelectedValue;
            ScheduleTask.ItemsSource = automation.State.Tasks.ToArray(); ScheduleTask.SelectedValue = selected;
            Parallel.SelectedIndex = automation.State.ParallelJobs - 1;
            PauseButton.Content = automation.State.Paused ? L.T("Reanudar cola") : L.T("Pausar cola");
            Summary.Text = automation.BlockReason ?? $"{automation.State.Tasks.Count} tareas guardadas · {automation.State.Queue.Count(item => item.State == "waiting")} en cola · {automation.State.Queue.Count(item => item.State == "running")} en curso · {(automation.State.Paused ? L.T("Cola pausada") : L.T("Cola activa"))}";
            ShowLog();
        } finally { refreshing = false; }
    }
    private static void RefreshGrid<T>(DataGrid grid, IEnumerable<T> items, Func<object, string> id)
    {
        var selected = grid.SelectedItem is null ? null : id(grid.SelectedItem);
        grid.ItemsSource = items.ToArray();
        if (selected is not null) grid.SelectedItem = grid.Items.Cast<object>().FirstOrDefault(item => id(item) == selected);
    }
    private void Action(Action operation, string? success = null)
    {
        try { if (automation is null) throw new IOException(L.T("Espera a que se preparen los componentes.")); operation(); Refresh(); if (success is not null) Feedback.Text = success; }
        catch (Exception error) { Feedback.Text = error.Message; }
    }
    private RcloneTaskDefinition SelectedTask() => TasksGrid.SelectedItem as RcloneTaskDefinition ?? throw new ArgumentException(L.T("Selecciona una tarea guardada."));
    private RcloneQueueItem SelectedQueue() => QueueGrid.SelectedItem as RcloneQueueItem ?? throw new ArgumentException(L.T("Selecciona una operación de la cola."));
    private RcloneSchedule SelectedSchedule() => SchedulesGrid.SelectedItem as RcloneSchedule ?? throw new ArgumentException(L.T("Selecciona un horario."));
    private static string SelectedTag(ComboBox box) => ((ComboBoxItem)box.SelectedItem).Tag.ToString()!;
    private static void Select(ComboBox box, string value) => box.SelectedItem = box.Items.Cast<ComboBoxItem>().Single(item => item.Tag.ToString() == value);
    private static int Number(TextBox box) => int.TryParse(box.Text, NumberStyles.Integer, CultureInfo.InvariantCulture, out var value) ? value : throw new ArgumentException(L.T("Introduce un número entero en las opciones de transferencia."));
    private RcloneTaskDefinition ReadTask() => new() {
        RunAtStartup = SelectedTag(Operation) == "mount" && MountStartup.IsChecked == true, MountReadOnly = MountReadOnly.IsChecked == true,
        MountCacheMode = SelectedTag(MountCacheMode), MountCacheSize = MountCacheSize.Text.Trim(), MountCacheAge = MountCacheAge.Text.Trim(),
        Id = editingTask ?? Guid.NewGuid().ToString("N"), Name = TaskName.Text.Trim(), Direction = SelectedTag(Direction), Operation = SelectedTag(Operation), RemoteName = TaskRemote.Text.Trim(), Source = Source.Text.Trim(), Destination = Destination.Text.Trim(),
        Transfers = Number(Transfers), Checkers = Number(Checkers), Retries = Number(Retries), LowLevelRetries = Number(LowRetries), ConnectTimeout = Number(ConnectTimeout), IdleTimeout = Number(IdleTimeout), MaxDepth = Number(MaxDepth),
        Bandwidth = Bandwidth.Text.Trim(), MinSize = MinSize.Text.Trim(), MaxSize = MaxSize.Text.Trim(), MinAge = MinAge.Text.Trim(), MaxAge = MaxAge.Text.Trim(), Compare = SelectedTag(Compare), DeleteMode = SelectedTag(DeleteMode),
        Immutable = Immutable.IsChecked == true, SkipExisting = SkipExisting.IsChecked == true, SkipNewer = SkipNewer.IsChecked == true, OneFileSystem = OneFileSystem.IsChecked == true, NoUpdateModTime = NoModTime.IsChecked == true,
        DeleteExcluded = DeleteExcluded.IsChecked == true, AllowDestructive = AllowDestructive.IsChecked == true, Includes = Includes.Text, Excludes = Excludes.Text
    };
    private void LoadTask(RcloneTaskDefinition item, bool isNew = false)
    {
        editingTask = isNew ? null : item.Id; EditorTitle.Text = isNew ? L.T("Nueva tarea") : L.T("Editar tarea");
        TaskName.Text = item.Name; TaskRemote.Text = item.RemoteName; Source.Text = item.Source; Destination.Text = item.Destination; Select(Direction, item.Direction); Select(Operation, item.Operation);
        Source.Text = item.Source; Destination.Text = item.Destination;
        MountStartup.IsChecked = item.RunAtStartup; MountReadOnly.IsChecked = item.MountReadOnly; Select(MountCacheMode, item.MountCacheMode); MountCacheSize.Text = item.MountCacheSize; MountCacheAge.Text = item.MountCacheAge;
        Transfers.Text = item.Transfers.ToString(); Checkers.Text = item.Checkers.ToString(); Retries.Text = item.Retries.ToString(); LowRetries.Text = item.LowLevelRetries.ToString(); ConnectTimeout.Text = item.ConnectTimeout.ToString(); IdleTimeout.Text = item.IdleTimeout.ToString(); MaxDepth.Text = item.MaxDepth.ToString();
        Bandwidth.Text = item.Bandwidth; MinSize.Text = item.MinSize; MaxSize.Text = item.MaxSize; MinAge.Text = item.MinAge; MaxAge.Text = item.MaxAge; Select(Compare, item.Compare); Select(DeleteMode, item.DeleteMode);
        Immutable.IsChecked = item.Immutable; SkipExisting.IsChecked = item.SkipExisting; SkipNewer.IsChecked = item.SkipNewer; OneFileSystem.IsChecked = item.OneFileSystem; NoModTime.IsChecked = item.NoUpdateModTime;
        DeleteExcluded.IsChecked = item.DeleteExcluded; AllowDestructive.IsChecked = item.AllowDestructive; Includes.Text = item.Includes; Excludes.Text = item.Excludes;
    }
    private void NewTaskClick(object sender, RoutedEventArgs e) => Action(() => LoadTask(new(), true));
    private void OperationChanged(object sender, SelectionChangedEventArgs e)
    {
        if (MountOptions is null || Operation.SelectedItem is null) return;
        var mount = SelectedTag(Operation) == "mount";
        MountOptions.Visibility = mount ? Visibility.Visible : Visibility.Collapsed;
        TransferOptions.Visibility = FilterOptions.Visibility = LocalPickers.Visibility = mount ? Visibility.Collapsed : Visibility.Visible;
        Direction.IsEnabled = !mount;
        SourceLabel.Text = mount ? L.T("Origen · carpeta remota (ej. /)") : L.T("Origen · ruta local absoluta o ruta en la conexión remota, según dirección");
        DestinationLabel.Text = mount ? L.T("Destino · letra de unidad libre (ej. T:)") : L.T("Destino · carpeta remota o carpeta local, según dirección");
        if (mount) { if (string.IsNullOrWhiteSpace(Source.Text) || Path.IsPathFullyQualified(Source.Text)) Source.Text = "/"; if (Destination.Text == "/") Destination.Text = "T:"; }
    }
    private void EditTaskClick(object sender, RoutedEventArgs e) => Action(() => LoadTask(SelectedTask()));
    private void DuplicateTaskClick(object sender, RoutedEventArgs e) => Action(() => LoadTask(SelectedTask() with { Name = SelectedTask().Name + " (copia)" }, true));
    private void SaveTaskClick(object sender, RoutedEventArgs e) => Action(() => { var task = ReadTask(); automation!.SaveTask(task); editingTask = task.Id; EditorTitle.Text = L.T("Editar tarea"); }, L.T("Tarea guardada. Ya puedes añadirla a la cola o programarla."));
    private void DeleteTaskClick(object sender, RoutedEventArgs e) => Action(() => {
        var task = SelectedTask();
        if (MessageBox.Show(L.T("\u00BFEliminar \u00AB{0}\u00BB y sus horarios? Las operaciones ya a\u00F1adidas a la cola se conservan.", task.Name), L.T("Eliminar tarea"), MessageBoxButton.YesNo, MessageBoxImage.Question) != MessageBoxResult.Yes) return;
        automation!.DeleteTask(task.Id); if (editingTask == task.Id) LoadTask(new(), true);
    });
    private void RunTaskClick(object sender, RoutedEventArgs e) => Action(() => automation!.Enqueue(SelectedTask()), L.T("Tarea añadida a la cola."));
    private void DryRunTaskClick(object sender, RoutedEventArgs e) => Action(() => automation!.Enqueue(SelectedTask(), true), L.T("Simulación añadida a la cola; rclone no modificará los archivos."));
    private void ChooseFolderClick(object sender, RoutedEventArgs e)
    {
        var picker = new Microsoft.Win32.OpenFolderDialog { Title = L.T("Carpeta local de la tarea") };
        if (picker.ShowDialog(Window.GetWindow(this)) == true) { if (SelectedTag(Direction) == "upload") Source.Text = picker.FolderName; else Destination.Text = picker.FolderName; }
    }
    private void ChooseFileClick(object sender, RoutedEventArgs e)
    {
        if (SelectedTag(Direction) != "upload") { Feedback.Text = L.T("Para descargar, selecciona una carpeta local como destino."); return; }
        var picker = new Microsoft.Win32.OpenFileDialog { Title = L.T("Archivo local para subir") };
        if (picker.ShowDialog(Window.GetWindow(this)) == true) Source.Text = picker.FileName;
    }
    private void ExportTasksClick(object sender, RoutedEventArgs e) => Action(() => {
        var picker = new Microsoft.Win32.SaveFileDialog { Filter = L.T("Tareas de rclone (*.json)|*.json"), FileName = "Teldrive-tareas.json" };
        if (picker.ShowDialog(Window.GetWindow(this)) == true) File.WriteAllText(picker.FileName, JsonSerializer.Serialize(automation!.State.Tasks.Select(item => item with { AllowDestructive = false }), new JsonSerializerOptions { WriteIndented = true }));
    });
    private void ImportTasksClick(object sender, RoutedEventArgs e) => Action(() => {
        var picker = new Microsoft.Win32.OpenFileDialog { Filter = L.T("Tareas de rclone (*.json)|*.json") };
        if (picker.ShowDialog(Window.GetWindow(this)) != true) return;
        if (new FileInfo(picker.FileName).Length > 2 * 1024 * 1024) throw new IOException(L.T("El archivo de tareas supera 2 MB."));
        var items = JsonSerializer.Deserialize<List<RcloneTaskDefinition>>(File.ReadAllText(picker.FileName)) ?? throw new IOException(L.T("No se encontraron tareas."));
        if (items.Count > 1000) throw new IOException(L.T("Importa como máximo 1000 tareas a la vez."));
        var validated = items.Select(item => item with { Id = Guid.NewGuid().ToString("N"), AllowDestructive = false, RunAtStartup = false }).ToArray();
        foreach (var item in validated) _ = item.Arguments(item.IsMount ? false : true, false);
        foreach (var item in validated) automation!.SaveTask(item);
        Feedback.Text = L.T("Tareas importadas. Revisa las rutas y activa de nuevo cualquier eliminación o montaje al iniciar.");
    });
    private void PauseClick(object sender, RoutedEventArgs e) => Action(() => automation!.ConfigureQueue(!automation.State.Paused, automation.State.ParallelJobs));
    private void ParallelChanged(object sender, SelectionChangedEventArgs e) { if (!refreshing && automation is not null && Parallel.SelectedIndex >= 0) ApplyQueueClick(sender, e); }
    private void ApplyQueueClick(object sender, RoutedEventArgs e) => Action(() => automation!.ConfigureQueue(automation.State.Paused, Parallel.SelectedIndex + 1));
    private void MoveUpClick(object sender, RoutedEventArgs e) => Action(() => automation!.Move(SelectedQueue().Id, -1));
    private void MoveDownClick(object sender, RoutedEventArgs e) => Action(() => automation!.Move(SelectedQueue().Id, 1));
    private async void CancelClick(object sender, RoutedEventArgs e)
    {
        try { await automation!.CancelAsync(SelectedQueue().Id); Refresh(); }
        catch (Exception error) { Feedback.Text = error.Message; }
    }
    private void RetryClick(object sender, RoutedEventArgs e) => Action(() => automation!.Retry(SelectedQueue().Id), L.T("Nueva ejecución añadida a la cola."));
    private void RemoveClick(object sender, RoutedEventArgs e) => Action(() => automation!.Remove(SelectedQueue().Id));
    private void ClearHistoryClick(object sender, RoutedEventArgs e) => Action(() => automation!.ClearHistory());
    private void QueueSelectionChanged(object sender, SelectionChangedEventArgs e) { if (!refreshing) ShowLog(); }
    private void ShowLog() { var log = (QueueGrid.SelectedItem as RcloneQueueItem)?.Log ?? L.T("Selecciona una operación para ver su registro."); if (QueueLog.Text != log) { QueueLog.Text = log; QueueLog.ScrollToEnd(); } }
    private void ExportLogClick(object sender, RoutedEventArgs e) => Action(() => {
        var item = SelectedQueue(); var picker = new Microsoft.Win32.SaveFileDialog { Filter = L.T("Registro (*.txt)|*.txt"), FileName = "Teldrive-registro.txt" };
        if (picker.ShowDialog(Window.GetWindow(this)) == true) File.WriteAllText(picker.FileName, item.Log);
    });
    private void NewScheduleClick(object sender, RoutedEventArgs e) => Action(() => { editingSchedule = null; ScheduleDate.SelectedDate = DateTime.Today.AddDays(1); ScheduleTime.Text = "09:00"; Select(Repeat, "once"); Interval.Text = "60"; RunMissed.IsChecked = false; });
    private void EditScheduleClick(object sender, RoutedEventArgs e) => Action(() => {
        var schedule = SelectedSchedule(); editingSchedule = schedule.Id; ScheduleTask.SelectedValue = schedule.TaskId;
        ScheduleDate.SelectedDate = schedule.NextRun.LocalDateTime.Date; ScheduleTime.Text = schedule.NextRun.ToLocalTime().ToString("HH:mm"); Select(Repeat, schedule.Repeat); Interval.Text = schedule.IntervalMinutes.ToString(); RunMissed.IsChecked = schedule.RunMissed;
    });
    private void SaveScheduleClick(object sender, RoutedEventArgs e) => Action(() => {
        if (ScheduleDate.SelectedDate is not DateTime date || !TimeSpan.TryParseExact(ScheduleTime.Text, @"hh\:mm", CultureInfo.InvariantCulture, out var time)) throw new ArgumentException(L.T("Selecciona la fecha y una hora válida en formato HH:mm."));
        var local = DateTime.SpecifyKind(date.Date + time, DateTimeKind.Unspecified);
        if (TimeZoneInfo.Local.IsInvalidTime(local)) throw new ArgumentException(L.T("Esa hora no existe debido al cambio de horario. Elige otra hora."));
        var task = automation!.State.Tasks.SingleOrDefault(item => item.Id == ScheduleTask.SelectedValue as string) ?? throw new ArgumentException(L.T("Elige una tarea guardada."));
        var schedule = new RcloneSchedule { Id = editingSchedule ?? Guid.NewGuid().ToString("N"), TaskId = task.Id, Name = task.Name, Repeat = SelectedTag(Repeat), IntervalMinutes = Number(Interval), RunMissed = RunMissed.IsChecked == true, NextRun = new DateTimeOffset(TimeZoneInfo.ConvertTimeToUtc(local, TimeZoneInfo.Local), TimeSpan.Zero) };
        automation.SaveSchedule(schedule); editingSchedule = schedule.Id;
    }, L.T("Horario guardado."));
    private void ToggleScheduleClick(object sender, RoutedEventArgs e) => Action(() => automation!.ToggleSchedule(SelectedSchedule().Id));
    private void DeleteScheduleClick(object sender, RoutedEventArgs e) => Action(() => automation!.DeleteSchedule(SelectedSchedule().Id));
    private void SavePreferencesClick(object sender, RoutedEventArgs e) => Action(() => savePreferences!(new() { Language = LanguageChoice.SelectedValue as string ?? "es", AlwaysShowInTray = AlwaysTray.IsChecked == true, MinimizeToTray = MinimizeTray.IsChecked == true, CloseToTray = CloseTray.IsChecked == true, StartInTray = StartTray.IsChecked == true, StartWithWindows = StartWindows.IsChecked == true, NotifyFinished = NotifyFinished.IsChecked == true }), L.T("Preferencias guardadas."));
    private void HideClick(object sender, RoutedEventArgs e) => hideWindow?.Invoke();
    private void ExitClick(object sender, RoutedEventArgs e) => exit?.Invoke();
}
