package io.webr0m.teldrive;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.DownloadManager;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.WindowInsets;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;
import java.net.URI;
import java.net.URISyntaxException;
import androidx.webkit.WebViewCompat;
import androidx.webkit.WebViewFeature;
import org.json.JSONObject;
import java.util.Collections;

public final class MainActivity extends Activity {
    private static final int FILE_PICKER = 41;
    private static final int BLUE = 0xff1a73e8;
    private static final int BG = 0xfff6f8fc;
    private LinearLayout root;
    private FrameLayout content;
    private WebView web;
    private ProgressBar progress;
    private TextView title;
    private SharedPreferences preferences;
    private ValueCallback<Uri[]> upload;
    private URI server;
    private View fullscreen;
    private WebChromeClient.CustomViewCallback fullscreenCallback;
    private String[] pendingDownload;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        preferences = getSharedPreferences("connection", MODE_PRIVATE);
        root = new LinearLayout(this); root.setOrientation(LinearLayout.VERTICAL); root.setBackgroundColor(BG);
        root.setOnApplyWindowInsetsListener((view, insets) -> {
            if (android.os.Build.VERSION.SDK_INT >= 30) { android.graphics.Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout()); view.setPadding(bars.left, bars.top, bars.right, bars.bottom); }
            else view.setPadding(insets.getSystemWindowInsetLeft(), insets.getSystemWindowInsetTop(), insets.getSystemWindowInsetRight(), insets.getSystemWindowInsetBottom());
            return insets;
        });
        LinearLayout toolbar = new LinearLayout(this); toolbar.setGravity(Gravity.CENTER_VERTICAL); toolbar.setPadding(dp(12), 0, dp(8), 0);
        title = text("☁  Teldrive", 20, Color.rgb(29, 42, 62)); toolbar.addView(title, new LinearLayout.LayoutParams(0, dp(56), 1));
        Button refresh = button("↻", false); refresh.setContentDescription("Actualizar unidad"); refresh.setOnClickListener(v -> { if (web != null && web.getVisibility() == View.VISIBLE) web.reload(); }); toolbar.addView(refresh, new LinearLayout.LayoutParams(dp(48), dp(48)));
        Button settings = button("⋮", false); settings.setContentDescription("Opciones de Teldrive"); settings.setOnClickListener(v -> menu()); toolbar.addView(settings, new LinearLayout.LayoutParams(dp(48), dp(48)));
        root.addView(toolbar);
        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal); progress.setMax(100); progress.setVisibility(View.INVISIBLE); root.addView(progress, new LinearLayout.LayoutParams(-1, dp(3)));
        content = new FrameLayout(this); root.addView(content, new LinearLayout.LayoutParams(-1, 0, 1)); setContentView(root);
        if (android.os.Build.VERSION.SDK_INT >= 33) getOnBackInvokedDispatcher().registerOnBackInvokedCallback(android.window.OnBackInvokedDispatcher.PRIORITY_DEFAULT, this::back);
        if (preferences.contains("server")) {
            try { connect(ServerAddress.parse(preferences.getString("server", ""), preferences.getBoolean("localHttp", false)), false); }
            catch (IllegalArgumentException ignored) { showConnection(); }
        } else showConnection();
    }

    private int dp(int value) { return Math.round(value * getResources().getDisplayMetrics().density); }
    private TextView text(String value, int size, int color) { TextView view = new TextView(this); view.setText(value); view.setTextSize(size); view.setTextColor(color); view.setGravity(Gravity.CENTER_VERTICAL); return view; }
    private Button button(String label, boolean filled) { Button view = new Button(this); view.setText(label); view.setAllCaps(false); view.setMinHeight(dp(48)); view.setTextColor(filled ? Color.WHITE : BLUE); GradientDrawable shape = new GradientDrawable(); shape.setColor(filled ? BLUE : Color.TRANSPARENT); shape.setCornerRadius(dp(12)); view.setBackground(shape); return view; }
    private void showConnection() {
        if (web != null) web.setVisibility(View.GONE);
        content.removeViews(0, content.getChildCount());
        ScrollView scroll = new ScrollView(this); LinearLayout form = new LinearLayout(this); form.setOrientation(LinearLayout.VERTICAL); form.setPadding(dp(24), dp(24), dp(24), dp(24));
        form.addView(text("Tu unidad, donde estés", 28, Color.rgb(29, 42, 62)));
        TextView intro = text("Conecta tu servidor Teldrive v2 para abrir tus archivos, escuchar música, ver vídeos y compartir contenido.", 16, 0xff667085); intro.setPadding(0, dp(12), 0, dp(28)); form.addView(intro);
        form.addView(text("Dirección del servidor", 14, 0xff344054));
        EditText address = new EditText(this); address.setSingleLine(); address.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI); address.setHint("https://teldrive.tudominio.com"); address.setText(preferences.getString("server", "")); address.setMinHeight(dp(56)); form.addView(address);
        CheckBox local = new CheckBox(this); local.setText("Permitir HTTP en mi red local"); local.setMinHeight(dp(48)); local.setChecked(preferences.getBoolean("localHttp", false)); form.addView(local);
        TextView error = text("", 14, 0xffb42318); error.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE); form.addView(error);
        Button connect = button("Conectar a mi unidad", true); form.addView(connect, new LinearLayout.LayoutParams(-1, dp(52)));
        connect.setOnClickListener(v -> {
            try { URI next = ServerAddress.parse(address.getText().toString(), local.isChecked()); preferences.edit().putString("server", next.toString()).putBoolean("localHttp", local.isChecked()).apply(); connect(next, true); }
            catch (IllegalArgumentException failure) { error.setText(failure.getMessage()); }
        });
        TextView help = text("Usa la dirección accesible desde tu móvil. 127.0.0.1 apunta al propio teléfono. El servidor y sus archivos permanecen en tu equipo o servidor; esta app no necesita PostgreSQL, rclone ni WinFsp.", 14, 0xff667085); help.setPadding(0, dp(24), 0, dp(24)); form.addView(help);
        if (server != null) { Button back = button("Volver a mi unidad", false); back.setOnClickListener(v -> connect(server, false)); form.addView(back); }
        scroll.addView(form); content.addView(scroll);
        title.setText("☁  Teldrive");
    }

    @android.annotation.SuppressLint("SetJavaScriptEnabled")
    private void connect(URI address, boolean fresh) {
        server = address;
        content.removeAllViews();
        if (web == null) {
            web = new WebView(this);
            WebSettings config = web.getSettings(); config.setJavaScriptEnabled(true); config.setDomStorageEnabled(true); config.setMediaPlaybackRequiresUserGesture(true); config.setAllowFileAccess(false); config.setAllowContentAccess(true); config.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW); config.setSupportZoom(false); config.setSafeBrowsingEnabled(true);
            CookieManager.getInstance().setAcceptCookie(true); CookieManager.getInstance().setAcceptThirdPartyCookies(web, false);
            WebView.setWebContentsDebuggingEnabled(BuildConfig.DEBUG);
            web.setWebViewClient(new WebViewClient() {
                @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                    try { URI target = new URI(request.getUrl().toString()); if (ServerAddress.sameOrigin(server, target)) return false; if (request.hasGesture() && request.isForMainFrame()) external(request.getUrl()); }
                    catch (URISyntaxException | NullPointerException ignored) {}
                    return true;
                }
                @Override public void onPageFinished(WebView view, String url) { CookieManager.getInstance().flush(); progress.setVisibility(View.INVISIBLE); title.setText("☁  Teldrive"); }
                @Override public boolean onRenderProcessGone(WebView view, android.webkit.RenderProcessGoneDetail detail) {
                    content.removeView(view); view.destroy(); web = null;
                    if (upload != null) { upload.onReceiveValue(null); upload = null; }
                    progress.setVisibility(View.INVISIBLE); showConnection();
                    Toast.makeText(MainActivity.this, "La vista se cerró. Conecta de nuevo para continuar.", Toast.LENGTH_LONG).show(); return true;
                }
                @Override public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) { if (request.isForMainFrame()) connectionError("No se pudo conectar al servidor. Comprueba la dirección y tu conexión."); }
                @Override public void onReceivedHttpError(WebView view, WebResourceRequest request, WebResourceResponse response) { if (request.isForMainFrame() && response.getStatusCode() >= 500) connectionError("El servidor no está disponible. Vuelve a intentarlo."); }
                // Certificate failures use WebView's default cancellation; there is no bypass.
            });
            web.setWebChromeClient(new WebChromeClient() {
                @Override public void onProgressChanged(WebView view, int value) { progress.setProgress(value); progress.setVisibility(value < 100 ? View.VISIBLE : View.INVISIBLE); }
                @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                    if (upload != null) upload.onReceiveValue(null); upload = callback;
                    Intent choose = new Intent(Intent.ACTION_OPEN_DOCUMENT); choose.addCategory(Intent.CATEGORY_OPENABLE); choose.setType("*/*"); choose.putExtra(Intent.EXTRA_ALLOW_MULTIPLE, params.getMode() == FileChooserParams.MODE_OPEN_MULTIPLE);
                    String[] types = params.getAcceptTypes(); if (types != null && types.length > 0 && !types[0].isEmpty()) choose.putExtra(Intent.EXTRA_MIME_TYPES, types);
                    try { startActivityForResult(Intent.createChooser(choose, "Archivos para Teldrive"), FILE_PICKER); }
                    catch (ActivityNotFoundException error) { upload.onReceiveValue(null); upload = null; Toast.makeText(MainActivity.this, "No se encontró un selector de archivos.", Toast.LENGTH_LONG).show(); }
                    return true;
                }
                @Override public void onShowCustomView(View view, CustomViewCallback callback) {
                    if (fullscreen != null) { callback.onCustomViewHidden(); return; } fullscreen = view; fullscreenCallback = callback; root.setVisibility(View.GONE); addContentView(view, new FrameLayout.LayoutParams(-1, -1));
                }
                @Override public void onHideCustomView() { hideFullscreen(); }
            });
            web.setDownloadListener((url, agent, disposition, mime, length) -> download(url, agent, disposition, mime));
        }
        if (WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)) {
            WebViewCompat.removeWebMessageListener(web, "teldriveMobile");
            WebViewCompat.addWebMessageListener(web, "teldriveMobile", Collections.singleton(server.getScheme() + "://" + server.getRawAuthority()), (view, message, origin, mainFrame, reply) -> {
                if (!mainFrame || !ServerAddress.sameOrigin(server, URI.create(origin.toString()))) return;
                String id = "";
                try {
                    String raw = message.getData(); if (raw == null || raw.length() > 16384) throw new IllegalArgumentException("Solicitud no válida.");
                    JSONObject request = new JSONObject(raw); id = request.getString("id");
                    if (!"download".equals(request.getString("method"))) throw new IllegalArgumentException("Operación no admitida.");
                    URI target = URI.create(request.getString("url"));
                    if (!ServerAddress.sameOrigin(server, target) || !target.getPath().matches("/api/v1/(files/[^/]+/content/[^/]+|public/shares/[^/]+/(files/[^/]+/)?content/[^/]+)")) throw new IllegalArgumentException("Solo se descargan archivos de tu servidor.");
                    JSONObject headers = request.optJSONObject("headers"); String password = headers == null ? "" : headers.optString("x-share-password", headers.optString("X-Share-Password", ""));
                    if (password.contains("\r") || password.contains("\n")) throw new IllegalArgumentException("La contraseña del enlace no es válida.");
                    enqueue(target.toString(), web.getSettings().getUserAgentString(), request.getString("filename"), "application/octet-stream", password);
                    reply.postMessage(new JSONObject().put("id", id).put("result", true).toString());
                } catch (Exception error) { try { reply.postMessage(new JSONObject().put("id", id).put("error", "No se pudo iniciar la descarga en el dispositivo.").toString()); } catch (Exception ignored) {} }
            });
        }
        web.setVisibility(View.VISIBLE); content.addView(web, new FrameLayout.LayoutParams(-1, -1));
        if (fresh || web.getUrl() == null || !web.getUrl().startsWith(server.toString())) web.loadUrl(server.toString());
    }

    private void connectionError(String message) {
        progress.setVisibility(View.INVISIBLE);
        new AlertDialog.Builder(this).setTitle("Conexión con Teldrive").setMessage(message).setPositiveButton("Reintentar", (dialog, which) -> web.reload()).setNegativeButton("Configurar", (dialog, which) -> showConnection()).show();
    }
    private void menu() {
        new AlertDialog.Builder(this).setTitle("Teldrive").setItems(new String[]{"Mi unidad", "Apariencia", "Descargas del dispositivo", "Cambiar servidor", "Cerrar sesión y borrar datos locales"}, (dialog, choice) -> {
            if (choice == 0 || choice == 1) { if (server == null) showConnection(); else { connect(server, false); web.loadUrl(server.resolve(choice == 0 ? "/files" : "/settings/appearance").toString()); } }
            else if (choice == 2) { try { startActivity(new Intent(DownloadManager.ACTION_VIEW_DOWNLOADS)); } catch (ActivityNotFoundException ignored) { Toast.makeText(this, "Abre la carpeta Descargas de tu dispositivo.", Toast.LENGTH_LONG).show(); } }
            else if (choice == 3) showConnection();
            else new AlertDialog.Builder(this).setTitle("¿Borrar los datos de esta app?").setMessage("Se eliminarán la sesión y las preferencias del dispositivo. Tus archivos del servidor se conservarán.").setPositiveButton("Borrar", (d, w) -> { CookieManager.getInstance().removeAllCookies(null); android.webkit.WebStorage.getInstance().deleteAllData(); if (web != null) { web.clearCache(true); web.clearHistory(); web.destroy(); web = null; } server = null; preferences.edit().clear().apply(); showConnection(); }).setNegativeButton("Cancelar", null).show();
        }).show();
    }
    private void external(Uri uri) {
        String scheme = uri.getScheme(); if (!("https".equalsIgnoreCase(scheme) || "http".equalsIgnoreCase(scheme) || "tg".equalsIgnoreCase(scheme))) return;
        try { startActivity(new Intent(Intent.ACTION_VIEW, uri)); } catch (ActivityNotFoundException ignored) { Toast.makeText(this, "No hay una aplicación para abrir este enlace.", Toast.LENGTH_LONG).show(); }
    }
    private void download(String url, String agent, String disposition, String mime) {
        try {
            URI address = new URI(url);
            if (!ServerAddress.sameOrigin(server, address)) { Toast.makeText(this, "Solo se descargan archivos de tu servidor.", Toast.LENGTH_LONG).show(); return; }
            String filename = android.webkit.URLUtil.guessFileName(url, disposition, mime).replaceAll("[\\\\/:*?\"<>|\\p{Cntrl}]", "_");
            enqueue(url, agent, filename, mime, "");
        } catch (Exception error) { Toast.makeText(this, "Esta descarga no se pudo iniciar. Usa el enlace de descarga del archivo.", Toast.LENGTH_LONG).show(); }
    }
    private void enqueue(String url, String agent, String name, String mime, String password) {
            if (android.os.Build.VERSION.SDK_INT < 29 && checkSelfPermission(android.Manifest.permission.WRITE_EXTERNAL_STORAGE) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                if (pendingDownload != null) throw new IllegalStateException("Espera a la autorización de la descarga anterior.");
                pendingDownload = new String[]{url, agent, name, mime, password};
                requestPermissions(new String[]{android.Manifest.permission.WRITE_EXTERNAL_STORAGE}, 61); return;
            }
            String filename = name.replaceAll("[\\\\/:*?\"<>|\\p{Cntrl}]", "_");
            if (filename.trim().isEmpty()) filename = "archivo";
            DownloadManager.Request request = new DownloadManager.Request(Uri.parse(url));
            if (!password.isEmpty()) request.addRequestHeader("X-Share-Password", password);
            String cookies = CookieManager.getInstance().getCookie(url); if (cookies != null) request.addRequestHeader("Cookie", cookies); request.addRequestHeader("User-Agent", agent); request.setTitle(filename); request.setMimeType(mime); request.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED); request.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, filename);
            ((DownloadManager)getSystemService(DOWNLOAD_SERVICE)).enqueue(request);
            Toast.makeText(this, "Descarga iniciada", Toast.LENGTH_SHORT).show();
    }
    @Override public void onRequestPermissionsResult(int requestCode, String[] permissions, int[] results) {
        super.onRequestPermissionsResult(requestCode, permissions, results);
        if (requestCode != 61 || pendingDownload == null) return;
        String[] request = pendingDownload; pendingDownload = null;
        if (results.length > 0 && results[0] == android.content.pm.PackageManager.PERMISSION_GRANTED) enqueue(request[0], request[1], request[2], request[3], request[4]);
        else Toast.makeText(this, "Autoriza Descargas para guardar archivos en Android 8 o 9.", Toast.LENGTH_LONG).show();
    }
    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request != FILE_PICKER || upload == null) return;
        Uri[] values = null;
        if (result == RESULT_OK && data != null) {
            ClipData clips = data.getClipData();
            if (clips != null) { values = new Uri[clips.getItemCount()]; for (int i = 0; i < values.length; i++) values[i] = clips.getItemAt(i).getUri(); }
            else if (data.getData() != null) values = new Uri[]{data.getData()};
        }
        upload.onReceiveValue(values); upload = null;
    }
    private void hideFullscreen() { if (fullscreen == null) return; ((android.view.ViewGroup)fullscreen.getParent()).removeView(fullscreen); fullscreen = null; root.setVisibility(View.VISIBLE); fullscreenCallback.onCustomViewHidden(); fullscreenCallback = null; }
    private void back() { if (fullscreen != null) hideFullscreen(); else if (web != null && web.getVisibility() == View.VISIBLE && web.canGoBack()) web.goBack(); else finish(); }
    @Override public void onBackPressed() { back(); }
    @Override protected void onPause() { if (web != null) web.onPause(); CookieManager.getInstance().flush(); super.onPause(); }
    @Override protected void onResume() { super.onResume(); if (web != null) web.onResume(); }
    @Override protected void onDestroy() { if (upload != null) { upload.onReceiveValue(null); upload = null; } if (web != null) { content.removeView(web); web.destroy(); } super.onDestroy(); }
}
