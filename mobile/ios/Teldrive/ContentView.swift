import SwiftUI
import WebKit

@MainActor
final class BrowserStore: ObservableObject {
    @Published var server: URL?
    @Published var error: String?
    @Published var downloading = false
    @Published var downloaded: DownloadedFile?
    let webView: WKWebView
    init() {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .default()
        configuration.allowsInlineMediaPlayback = true
        configuration.mediaTypesRequiringUserActionForPlayback = .all
        webView = WKWebView(frame: .zero, configuration: configuration)
        webView.allowsBackForwardNavigationGestures = true
        if let address = UserDefaults.standard.string(forKey: "server") {
            server = try? ServerAddress.parse(address, allowLocalHTTP: UserDefaults.standard.bool(forKey: "localHTTP"))
        }
    }
    func connect(_ url: URL) { server = url; error = nil; webView.load(URLRequest(url: url)) }
    func go(_ path: String) { guard let server, let url = URL(string: path, relativeTo: server)?.absoluteURL else { return }; webView.load(URLRequest(url: url)) }
    func clear() {
        webView.stopLoading()
        server = nil
        UserDefaults.standard.removeObject(forKey: "server")
        UserDefaults.standard.removeObject(forKey: "localHTTP")
        let store = WKWebsiteDataStore.default()
        store.removeData(ofTypes: WKWebsiteDataStore.allWebsiteDataTypes(), modifiedSince: .distantPast) {}
    }
}

struct DownloadedFile: Identifiable { let id = UUID(); let url: URL }

struct ContentView: View {
    @StateObject private var browser = BrowserStore()
    @State private var configuring = false
    @State private var serverText = UserDefaults.standard.string(forKey: "server") ?? ""
    @State private var localHTTP = UserDefaults.standard.bool(forKey: "localHTTP")
    @State private var connectionError: String?
    @State private var confirmClear = false

    var body: some View {
        NavigationStack {
            Group {
                if browser.server == nil || configuring { connectionForm }
                else { TeldriveWebView(store: browser).ignoresSafeArea(.container, edges: .bottom) }
            }
            .navigationTitle("Teldrive")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) { Image(systemName: "cloud.fill").foregroundStyle(.blue).accessibilityHidden(true) }
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button("Mi unidad", systemImage: "folder") { configuring = false; browser.go("/files") }
                        Button("Apariencia", systemImage: "paintpalette") { configuring = false; browser.go("/settings/appearance") }
                        Button("Actualizar", systemImage: "arrow.clockwise") { browser.webView.reload() }
                        Button("Cambiar servidor", systemImage: "server.rack") { configuring = true }
                        Button("Cerrar sesión y borrar datos locales", systemImage: "rectangle.portrait.and.arrow.right", role: .destructive) { confirmClear = true }
                    } label: { Image(systemName: "ellipsis.circle").frame(minWidth: 44, minHeight: 44) }
                    .accessibilityLabel("Opciones de Teldrive")
                }
            }
            .overlay(alignment: .top) { if browser.downloading { ProgressView("Descargando archivo…").padding(12).background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12)) } }
            .alert("Conexión con Teldrive", isPresented: Binding(get: { browser.error != nil }, set: { if !$0 { browser.error = nil } })) {
                Button("Reintentar") { browser.webView.reload() }
                Button("Configurar") { configuring = true }
            } message: { Text(browser.error ?? "") }
            .confirmationDialog("¿Borrar los datos de esta app?", isPresented: $confirmClear, titleVisibility: .visible) {
                Button("Borrar datos locales", role: .destructive) { browser.clear(); serverText = ""; localHTTP = false; configuring = true }
            } message: { Text("Se eliminarán la sesión y las preferencias del dispositivo. Tus archivos del servidor se conservarán.") }
            .sheet(item: $browser.downloaded) { file in ShareFile(url: file.url) }
        }
        .onAppear { if let server = browser.server, browser.webView.url == nil { browser.connect(server) } }
    }

    private var connectionForm: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                Image(systemName: "cloud.fill").font(.system(size: 52)).foregroundStyle(.blue).accessibilityHidden(true)
                Text("Tu unidad, donde estés").font(.largeTitle.bold())
                Text("Conecta tu servidor Teldrive v2 para abrir tus archivos, escuchar música, ver vídeos y compartir contenido.").foregroundStyle(.secondary)
                VStack(alignment: .leading, spacing: 12) {
                    Text("Dirección del servidor").font(.headline)
                    TextField("https://teldrive.tudominio.com", text: $serverText).textContentType(.URL).keyboardType(.URL).textInputAutocapitalization(.never).autocorrectionDisabled().padding(14).background(Color(.secondarySystemGroupedBackground), in: RoundedRectangle(cornerRadius: 12))
                    Toggle("Permitir HTTP en mi red local", isOn: $localHTTP).frame(minHeight: 44)
                    if let connectionError { Text(connectionError).foregroundStyle(.red).accessibilityAddTraits(.updatesFrequently) }
                    Button {
                        do {
                            let url = try ServerAddress.parse(serverText, allowLocalHTTP: localHTTP)
                            UserDefaults.standard.set(url.absoluteString, forKey: "server")
                            UserDefaults.standard.set(localHTTP, forKey: "localHTTP")
                            connectionError = nil; configuring = false; browser.connect(url)
                        } catch { connectionError = error.localizedDescription }
                    } label: { Text("Conectar a mi unidad").frame(maxWidth: .infinity, minHeight: 44) }
                    .buttonStyle(.borderedProminent)
                }
                Text("Usa una dirección accesible desde tu móvil. 127.0.0.1 apunta al propio teléfono. El servidor y sus archivos permanecen en tu equipo o servidor; esta app no necesita PostgreSQL, rclone ni WinFsp.").font(.footnote).foregroundStyle(.secondary)
                if browser.server != nil { Button("Volver a mi unidad") { configuring = false }.frame(minHeight: 44) }
            }
            .padding(24).frame(maxWidth: 620, alignment: .leading).frame(maxWidth: .infinity)
        }.background(Color(.systemGroupedBackground))
    }
}

struct ShareFile: UIViewControllerRepresentable {
    let url: URL
    func makeUIViewController(context: Context) -> UIActivityViewController { UIActivityViewController(activityItems: [url], applicationActivities: nil) }
    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}
