using System.IO;
using System.Text.Json;

namespace Teldrive.Desktop;

public sealed record RcloneJobStatus(string State, string Log, int? ExitCode);
public interface IRcloneExecutor
{
    string? BlockReason { get; }
    int AvailableSlots { get; }
    string StartDefinition(RcloneTaskDefinition definition, bool dryRun);
    RcloneJobStatus? Snapshot(string id);
    Task StopAsync(string id);
}

public sealed record RcloneSchedule
{
    public string Id { get; init; } = Guid.NewGuid().ToString("N");
    public string TaskId { get; init; } = "";
    public string Name { get; init; } = "";
    public string Repeat { get; init; } = "once";
    public int IntervalMinutes { get; init; } = 60;
    public DateTimeOffset NextRun { get; set; }
    public bool Enabled { get; set; } = true;
    public bool RunMissed { get; init; }
    public string LastResult { get; set; } = "";
    public string NextDisplay => Enabled ? NextRun.ToLocalTime().ToString("dd/MM/yyyy HH:mm") : "Desactivado";
    public string RepeatDisplay => Repeat switch { "daily" => "Diaria", "weekly" => "Semanal", "interval" => $"Cada {IntervalMinutes} min", _ => "Una vez" };

    public void Advance(DateTimeOffset now)
    {
        if (Repeat == "once") { Enabled = false; return; }
        if (Repeat == "interval") {
            var elapsed = Math.Max(0, (now - NextRun).TotalMinutes);
            NextRun = NextRun.AddMinutes((Math.Floor(elapsed / IntervalMinutes) + 1) * IntervalMinutes);
            return;
        }
        var local = TimeZoneInfo.ConvertTime(NextRun, TimeZoneInfo.Local).DateTime;
        var days = Repeat == "daily" ? 1 : 7;
        do {
            local = local.AddDays(days);
            // A nonexistent daylight-saving time runs at the first valid minute afterwards.
            while (TimeZoneInfo.Local.IsInvalidTime(local)) local = local.AddMinutes(1);
            NextRun = new DateTimeOffset(TimeZoneInfo.ConvertTimeToUtc(DateTime.SpecifyKind(local, DateTimeKind.Unspecified), TimeZoneInfo.Local), TimeSpan.Zero);
        } while (NextRun <= now);
    }
}

public sealed record RcloneQueueItem
{
    public string Id { get; init; } = Guid.NewGuid().ToString("N");
    public RcloneTaskDefinition Definition { get; init; } = new();
    public bool DryRun { get; init; }
    public string State { get; set; } = "waiting";
    public string? ProcessId { get; set; }
    public DateTimeOffset Created { get; init; } = DateTimeOffset.UtcNow;
    public DateTimeOffset? Started { get; set; }
    public DateTimeOffset? Finished { get; set; }
    public string Log { get; set; } = "";
    public string Name => Definition.Name + (DryRun ? " (simulación)" : "");
    public string StateDisplay => State switch { "waiting" => "En cola", "running" => "En curso", "completed" => "Completada", "failed" => "Fallida", "cancelled" => "Cancelada", "interrupted" => "Interrumpida; reintentar", _ => State };
    public string CreatedDisplay => Created.ToLocalTime().ToString("dd/MM HH:mm");
}

public sealed class RcloneAutomationState
{
    public int Version { get; set; } = 1;
    public List<RcloneTaskDefinition> Tasks { get; set; } = [];
    public List<RcloneQueueItem> Queue { get; set; } = [];
    public List<RcloneSchedule> Schedules { get; set; } = [];
    public bool Paused { get; set; }
    public int ParallelJobs { get; set; } = 1;
}

// Mutations and Tick run on the desktop dispatcher. Processes remain owned by RcloneRuntime.
public sealed class RcloneAutomation : IAsyncDisposable
{
    private readonly string path;
    private readonly IRcloneExecutor executor;
    private bool disposed;
    public RcloneAutomationState State { get; }
    public string? BlockReason => executor.BlockReason;
    public event Action<RcloneQueueItem>? Finished;

    public RcloneAutomation(string dataRoot, IRcloneExecutor executor)
    {
        this.executor = executor;
        path = Path.Combine(dataRoot, "rclone-automation.json");
        State = File.Exists(path) ? JsonSerializer.Deserialize<RcloneAutomationState>(File.ReadAllText(path)) ?? throw new IOException("El archivo de tareas está vacío.") : new();
        if (State.Version != 1 || State.ParallelJobs is < 1 or > 3 || State.Tasks is null || State.Queue is null || State.Schedules is null) throw new IOException("El archivo de tareas tiene un formato no compatible. Conserva una copia antes de corregirlo.");
        foreach (var item in State.Queue.Where(item => item.State == "running")) {
            item.State = "interrupted"; item.ProcessId = null; item.Finished = DateTimeOffset.UtcNow;
            item.Log += "\nEl programa se cerró durante esta operación. Reinténtala para continuar.\n";
        }
        Save();
    }

    public void SaveTask(RcloneTaskDefinition definition)
    {
        _ = definition.Arguments(true, false);
        var index = State.Tasks.FindIndex(item => item.Id == definition.Id);
        if (index >= 0) State.Tasks[index] = definition; else State.Tasks.Add(definition);
        State.Schedules = State.Schedules.Select(item => item.TaskId == definition.Id ? item with { Name = definition.Name } : item).ToList();
        Save();
    }
    public void DeleteTask(string id)
    {
        State.Tasks.RemoveAll(item => item.Id == id);
        State.Schedules.RemoveAll(item => item.TaskId == id);
        Save();
    }
    public RcloneQueueItem Enqueue(RcloneTaskDefinition definition, bool dryRun = false)
    {
        ObjectDisposedException.ThrowIf(disposed, this);
        _ = definition.Arguments(dryRun, false);
        if (State.Queue.Count(item => item.State is "waiting" or "running") >= 1000) throw new IOException("La cola alcanzó el límite de 1000 tareas pendientes.");
        var item = new RcloneQueueItem { Definition = definition, DryRun = dryRun };
        State.Queue.Add(item); TrimHistory(); Save(); return item;
    }
    public void Retry(string id)
    {
        var item = Find(id);
        if (item.State is "waiting" or "running") throw new ArgumentException("La tarea todavía está pendiente o en curso.");
        Enqueue(item.Definition, item.DryRun);
    }
    public void Move(string id, int direction)
    {
        var index = State.Queue.FindIndex(item => item.Id == id);
        if (index < 0 || State.Queue[index].State != "waiting") throw new ArgumentException("Solo se pueden ordenar tareas pendientes.");
        var other = index + Math.Sign(direction);
        while (other >= 0 && other < State.Queue.Count && State.Queue[other].State != "waiting") other += Math.Sign(direction);
        if (other < 0 || other >= State.Queue.Count) return;
        (State.Queue[index], State.Queue[other]) = (State.Queue[other], State.Queue[index]); Save();
    }
    public async Task CancelAsync(string id)
    {
        var item = Find(id);
        if (item.State is not ("waiting" or "running")) return;
        item.State = "cancelled"; item.Finished = DateTimeOffset.UtcNow; Save();
        if (item.ProcessId is not null) await executor.StopAsync(item.ProcessId);
    }
    public void Remove(string id)
    {
        if (Find(id).State == "running") throw new ArgumentException("Detén la tarea antes de quitarla.");
        State.Queue.RemoveAll(item => item.Id == id); Save();
    }
    public void ClearHistory() { State.Queue.RemoveAll(item => item.State is not ("waiting" or "running")); Save(); }
    public void ConfigureQueue(bool paused, int parallel)
    {
        if (parallel is < 1 or > 3) throw new ArgumentException("La cola admite entre una y tres tareas simultáneas.");
        State.Paused = paused; State.ParallelJobs = parallel; Save();
    }
    public void SaveSchedule(RcloneSchedule schedule)
    {
        var definition = State.Tasks.SingleOrDefault(item => item.Id == schedule.TaskId) ?? throw new ArgumentException("Elige una tarea guardada.");
        _ = definition.Arguments(false, false);
        if (schedule.Repeat is not ("once" or "daily" or "weekly" or "interval") || schedule.IntervalMinutes is < 1 or > 525600) throw new ArgumentException("Revisa la frecuencia del horario.");
        if (schedule.NextRun <= DateTimeOffset.UtcNow) throw new ArgumentException("Selecciona una fecha y hora futuras.");
        var index = State.Schedules.FindIndex(item => item.Id == schedule.Id);
        if (index < 0) State.Schedules.Add(schedule); else State.Schedules[index] = schedule;
        Save();
    }
    public void DeleteSchedule(string id) { State.Schedules.RemoveAll(item => item.Id == id); Save(); }
    public void ToggleSchedule(string id)
    {
        var schedule = State.Schedules.Single(item => item.Id == id);
        if (!schedule.Enabled && schedule.NextRun <= DateTimeOffset.UtcNow) throw new ArgumentException("Edita el horario y elige una fecha futura para reactivarlo.");
        schedule.Enabled = !schedule.Enabled; Save();
    }
    public void Tick(DateTimeOffset now)
    {
        if (disposed) return;
        var changed = false;
        foreach (var item in State.Queue.Where(item => item.State == "running").ToArray()) {
            var status = item.ProcessId is null ? null : executor.Snapshot(item.ProcessId);
            if (status is null) { item.State = "interrupted"; item.Log += "\nEl proceso dejó de estar disponible."; }
            else { item.Log = status.Log; if (status.State == "running") continue; item.State = status.State; }
            item.Finished = now; changed = true; Finished?.Invoke(item);
        }
        foreach (var schedule in State.Schedules.Where(item => item.Enabled && item.NextRun <= now).ToArray()) {
            var definition = State.Tasks.SingleOrDefault(item => item.Id == schedule.TaskId);
            var missed = now - schedule.NextRun > TimeSpan.FromMinutes(2) && !schedule.RunMissed;
            var duplicate = State.Queue.Any(item => item.Definition.Id == schedule.TaskId && item.State is "waiting" or "running");
            schedule.Advance(now); changed = true;
            schedule.LastResult = missed ? "Horario omitido: programa cerrado o equipo suspendido" : duplicate ? "Omitido: tarea pendiente o en curso" : definition is null ? "Tarea inexistente" : "Añadida a la cola";
            // Persist the next occurrence before admission; restarting cannot submit it twice.
            Save();
            if (definition is not null && !missed && !duplicate) {
                try { Enqueue(definition); }
                catch (Exception error) { schedule.LastResult = error.Message; }
            }
        }
        if (!State.Paused && executor.BlockReason is null) {
            var slots = Math.Min(executor.AvailableSlots, State.ParallelJobs - State.Queue.Count(item => item.State == "running"));
            foreach (var item in State.Queue.Where(item => item.State == "waiting").Take(Math.Max(0, slots)).ToArray()) {
                try { item.ProcessId = executor.StartDefinition(item.Definition, item.DryRun); item.State = "running"; item.Started = now; }
                catch (Exception error) { item.State = "failed"; item.Log = error.Message; item.Finished = now; Finished?.Invoke(item); }
                changed = true;
            }
        }
        if (changed) { TrimHistory(); Save(); }
    }
    private RcloneQueueItem Find(string id) => State.Queue.SingleOrDefault(item => item.Id == id) ?? throw new ArgumentException("La tarea ya no está en la cola.");
    private void TrimHistory()
    {
        foreach (var item in State.Queue.Where(item => item.State is not ("waiting" or "running")).OrderByDescending(item => item.Finished).Skip(200).ToArray()) State.Queue.Remove(item);
    }
    private void Save()
    {
        var temp = path + ".new";
        File.WriteAllText(temp, JsonSerializer.Serialize(State, new JsonSerializerOptions { WriteIndented = true }));
        File.Move(temp, path, true);
    }
    public async ValueTask DisposeAsync()
    {
        disposed = true;
        foreach (var item in State.Queue.Where(item => item.State == "running").ToArray()) {
            var snapshot = item.ProcessId is null ? null : executor.Snapshot(item.ProcessId);
            item.State = snapshot is not null && snapshot.State != "running" ? snapshot.State : "interrupted";
            item.Finished = DateTimeOffset.UtcNow;
            if (snapshot is not null) item.Log = snapshot.Log;
            if (item.ProcessId is not null && snapshot?.State == "running") await executor.StopAsync(item.ProcessId);
        }
        Save();
    }
}
