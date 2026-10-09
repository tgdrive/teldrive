package io.webr0m.teldrive;

import java.net.URI;
import java.net.URISyntaxException;
import java.util.Locale;

/** Restricts cleartext connections to explicit local-network addresses. */
public final class ServerAddress {
    private ServerAddress() {}
    public static URI parse(String value, boolean allowLocalHttp) {
        try {
            URI uri = new URI(value.trim());
            String host = uri.getHost();
            String scheme = uri.getScheme();
            if (host == null || uri.getUserInfo() != null || uri.getQuery() != null || uri.getFragment() != null
                    || !(uri.getPath() == null || uri.getPath().isEmpty() || uri.getPath().equals("/"))
                    || (uri.getPort() != -1 && (uri.getPort() < 1 || uri.getPort() > 65535))) throw new IllegalArgumentException("Introduce únicamente la dirección del servidor, sin credenciales ni rutas.");
            if (!"https".equalsIgnoreCase(scheme) && !("http".equalsIgnoreCase(scheme) && allowLocalHttp && isLocal(host)))
                throw new IllegalArgumentException("Utiliza HTTPS. Para una dirección privada de tu red, activa HTTP local.");
            return new URI(scheme.toLowerCase(Locale.ROOT), null, host.toLowerCase(Locale.ROOT), uri.getPort(), "/", null, null);
        } catch (URISyntaxException error) { throw new IllegalArgumentException("La dirección del servidor no es válida."); }
    }
    static boolean isLocal(String host) {
        host = host.toLowerCase(Locale.ROOT).replace("[", "").replace("]", "");
        if (host.equals("localhost") || host.equals("::1") || host.startsWith("fd") && host.contains(":") || host.startsWith("fc") && host.contains(":") || host.endsWith(".local")) return true;
        String[] parts = host.split("\\.", -1);
        if (parts.length != 4) return false;
        int[] octets = new int[4];
        try { for (int i = 0; i < 4; i++) { if (!parts[i].matches("[0-9]{1,3}")) return false; octets[i] = Integer.parseInt(parts[i]); if (octets[i] > 255) return false; } }
        catch (NumberFormatException ignored) { return false; }
        return octets[0] == 10 || octets[0] == 127 || octets[0] == 192 && octets[1] == 168 || octets[0] == 172 && octets[1] >= 16 && octets[1] <= 31;
    }
    public static boolean sameOrigin(URI first, URI second) {
        if (first == null || second == null || first.getScheme() == null || second.getScheme() == null || first.getHost() == null || second.getHost() == null) return false;
        return first.getScheme().equalsIgnoreCase(second.getScheme()) && first.getHost().equalsIgnoreCase(second.getHost()) && port(first) == port(second);
    }
    private static int port(URI uri) { return uri.getPort() >= 0 ? uri.getPort() : uri.getScheme().equalsIgnoreCase("https") ? 443 : 80; }
}
