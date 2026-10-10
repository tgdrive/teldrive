package io.webr0m.teldrive;

import org.junit.Test;
import java.net.URI;
import static org.junit.Assert.*;

public class ServerAddressTest {
    @Test public void httpsAndPrivateHttpAreNormalized() {
        assertEquals("https://example.com/", ServerAddress.parse("https://example.com", false).toString());
        assertEquals(8080, ServerAddress.parse("http://192.168.1.20:8080", true).getPort());
        assertTrue(ServerAddress.sameOrigin(URI.create("https://example.com"), URI.create("https://example.com:443/files")));
    }
    @Test public void unsafeServersAreRejected() {
        for (String value : new String[]{"http://example.com", "https://user:password@example.com", "https://example.com/files", "https://example.com?key=secret", "https://example.com:0", "http://10.0.foo.0.1", "http://10.0.0.1."}) assertThrows(value, IllegalArgumentException.class, () -> ServerAddress.parse(value, true));
        assertThrows(IllegalArgumentException.class, () -> ServerAddress.parse("http://192.168.1.20", false));
        assertFalse(ServerAddress.sameOrigin(URI.create("https://example.com"), URI.create("https://example.com:8443/files")));
        assertFalse(ServerAddress.sameOrigin(null, URI.create("https://example.com")));
        assertFalse(ServerAddress.sameOrigin(URI.create("https://example.com"), URI.create("blob:null/123")));
    }
}
