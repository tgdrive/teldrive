using System.IO;
using System.Reflection;
using System.Text;
using System.Text.Json;
using System.Windows.Controls;
using System.Windows.Input;
using Microsoft.Web.WebView2.Core;
using Microsoft.Web.WebView2.Wpf;

namespace Teldrive.Desktop;

public partial class MainWindow
{
    private async Task VerifyFolderInputAsync()
    {
        await InitializeBrowser();
        var core = Browser.CoreWebView2;
        core.AddWebResourceRequestedFilter(runtime.ServerUri!.GetLeftPart(UriPartial.Authority) + "/api/v1/*", CoreWebView2WebResourceContext.All);
        var requests = new List<string>();
        const string folderId = "11111111-1111-4111-8111-111111111111";
        void Fixture(object? sender, CoreWebView2WebResourceRequestedEventArgs e)
        {
            var uri = new Uri(e.Request.Uri);
            requests.Add(uri.AbsolutePath + uri.Query);
            var body = uri.AbsolutePath == "/api/v1/me"
                ? """{"userId":1,"displayName":"Input smoke","username":"fixture","premium":true,"role":"owner","capabilities":["files.read","files.write","files.share"]}"""
                : uri.AbsolutePath == "/api/v1/storage/stats"
                    ? """{"summary":{"logicalBytes":0,"trashBytes":0,"spamBytes":0,"activeFiles":0,"activeFolders":1,"trashedFiles":0},"growth":[],"categories":[],"channels":[],"cleanup":{"trashBytes":0,"staleUploadBytes":0,"staleUploads":0,"totalReclaimableBytes":0},"activity":[]}"""
                : uri.AbsolutePath == "/api/v1/files" && !uri.Query.Contains("parentId=")
                    ? """{"items":[{"id":"11111111-1111-4111-8111-111111111111","name":"Carpeta de prueba","kind":"folder","status":"active","generation":1,"modTime":"2026-10-12T00:00:00Z","createdAt":"2026-10-12T00:00:00Z","updatedAt":"2026-10-12T00:00:00Z","encryption":false}]}"""
                    : uri.AbsolutePath == "/api/v1/files" ? "{\"items\":[]}" : "{}";
            e.Response = core.Environment.CreateWebResourceResponse(new MemoryStream(Encoding.UTF8.GetBytes(body)), 200, "OK", "Content-Type: application/json\r\nCache-Control: no-store");
        }
        core.WebResourceRequested += Fixture;
        try
        {
            var sdkType = typeof(WebView2CompositionControl);
            var flags = BindingFlags.Instance | BindingFlags.NonPublic | BindingFlags.Public;
            var sdkBase = sdkType.GetField("m_webview2Base", flags)!.GetValue(Browser)!;
            var controller = (CoreWebView2CompositionController)sdkBase.GetType().GetProperty("CoreWebView2Controller", flags)!.GetValue(sdkBase)!;
            var locationField = sdkType.GetField("_mouselocation", flags)!;
            var results = new List<object>();
            foreach (var view in new[] { "list", "grid" })
            {
                var loaded = new TaskCompletionSource();
                void Navigated(object? sender, CoreWebView2NavigationCompletedEventArgs e) { if(e.IsSuccess) loaded.TrySetResult(); else loaded.TrySetException(new IOException("La navegación de prueba falló.")); }
                core.NavigationCompleted += Navigated;
                try { await ShowDrive("/files?view=" + view); await loaded.Task.WaitAsync(TimeSpan.FromSeconds(20)); }
                finally { core.NavigationCompleted -= Navigated; }
                double[]? point = null;
                for (var attempt = 0; attempt < 100 && point is null; attempt++)
                {
                    var json = await core.ExecuteScriptAsync("(() => {const row=document.querySelector('[role=row]');if(!row)return null;const name=row.querySelector('[title]')??row.querySelector('p')??row;const r=name.getBoundingClientRect();return [(r.left+r.width/2)*devicePixelRatio,(r.top+r.height/2)*devicePixelRatio]})()");
                    point = JsonSerializer.Deserialize<double[]>(json);
                    if (point is null) await Task.Delay(100);
                }
                if (point is null) {
                    await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "input-smoke-diagnostic.json"), JsonSerializer.Serialize(new { source=core.Source, requests, body=await core.ExecuteScriptAsync("document.body.innerText") }));
                    throw new IOException("No apareció la carpeta de prueba en " + view);
                }
                await core.ExecuteScriptAsync("window.inputSmokeEvents=[];for(const type of ['mousedown','mouseup','click','dblclick'])document.addEventListener(type,e=>window.inputSmokeEvents.push({type,detail:e.detail,target:e.target.tagName,selectionControl:Boolean(e.target.closest('label,button,[role=checkbox]'))}),true)");
                var position = new System.Drawing.Point((int)point[0], (int)point[1]);
                locationField.SetValue(Browser, position);
                controller.SendMouseInput(CoreWebView2MouseEventKind.Move, CoreWebView2MouseEventVirtualKeys.None, 0, position);
                controller.SendMouseInput(CoreWebView2MouseEventKind.LeftButtonDown, CoreWebView2MouseEventVirtualKeys.LeftButton, 0, position);
                controller.SendMouseInput(CoreWebView2MouseEventKind.LeftButtonUp, CoreWebView2MouseEventVirtualKeys.None, 0, position);
                await Task.Delay(70);
                // WPF raises MouseDoubleClick during the second MouseDown. The SDK
                // also forwards that MouseDown; test this native input sequence.
                if (arguments.Contains("--smoke-input-stock")) controller.SendMouseInput(CoreWebView2MouseEventKind.LeftButtonDoubleClick, CoreWebView2MouseEventVirtualKeys.LeftButton, 0, position);
                Browser.GetType().GetMethod("OnMouseDoubleClick", flags)!.Invoke(Browser, [new MouseButtonEventArgs(Mouse.PrimaryDevice, Environment.TickCount, MouseButton.Left) { RoutedEvent = Control.MouseDoubleClickEvent }]);
                controller.SendMouseInput(CoreWebView2MouseEventKind.LeftButtonDown, CoreWebView2MouseEventVirtualKeys.LeftButton, 0, position);
                controller.SendMouseInput(CoreWebView2MouseEventKind.LeftButtonUp, CoreWebView2MouseEventVirtualKeys.None, 0, position);
                for (var attempt = 0; attempt < 40 && !core.Source.Contains(folderId); attempt++) await Task.Delay(50);
                var events = await core.ExecuteScriptAsync("window.inputSmokeEvents");
                var opened = core.Source.Contains(folderId);
                results.Add(new { view, opened, events = JsonSerializer.Deserialize<JsonElement>(events) });
                if (!opened) { await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "input-smoke-events.json"), JsonSerializer.Serialize(results)); throw new IOException("El doble clic nativo no abrió la carpeta en " + view); }
            }
            await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "input-smoke-events.json"), JsonSerializer.Serialize(results));
            await File.WriteAllTextAsync(Path.Combine(runtime.DataRoot, "input-smoke-test.json"), JsonSerializer.Serialize(new { success = true, nativeMouseInput = true, list = true, grid = true }));
        }
        finally { core.WebResourceRequested -= Fixture; }
    }
}
