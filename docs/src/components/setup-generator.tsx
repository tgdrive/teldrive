import { useEffect, useMemo, useState } from 'react';

type Secrets = {
  signingKey: string;
  dataKey: string;
  encryptionKey: string;
};

type OutputKind = 'compose' | 'env' | 'yaml' | 'cli';

function randomBytes(length: number) {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  return bytes;
}

function base64Url(bytes: Uint8Array) {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
}

function generateSecrets(): Secrets {
  return {
    signingKey: base64Url(randomBytes(32)),
    dataKey: base64Url(randomBytes(32)),
    encryptionKey: base64Url(randomBytes(32)),
  };
}

const databasePassword = 'teldrive';

function buildCompose(secrets: Secrets, encryptionEnabled: boolean) {
  const encryption = encryptionEnabled
    ? `\n      TELDRIVE_ENCRYPTION_ACTIVE_KEY_VERSION: "1"\n      TELDRIVE_ENCRYPTION_KEYS: "1:${secrets.encryptionKey}"`
    : '';

  return `services:
  postgres:
    image: postgres:18-alpine
    restart: unless-stopped
    environment:
      POSTGRES_DB: teldrive
      POSTGRES_USER: teldrive
      POSTGRES_PASSWORD: ${databasePassword}
    volumes:
      - ./postgres-data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U teldrive -d teldrive"]
      interval: 5s
      timeout: 5s
      retries: 10

  teldrive:
    image: ghcr.io/tgdrive/teldrive:v2
    restart: unless-stopped
    depends_on:
      postgres:
        condition: service_healthy
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      TELDRIVE_HTTP_ADDRESS: "0.0.0.0:8080"
      TELDRIVE_DATABASE_URL: "postgres://teldrive:${databasePassword}@postgres:5432/teldrive?sslmode=disable"
      TELDRIVE_SECURITY_SIGNING_KEY: "${secrets.signingKey}"
      TELDRIVE_SECURITY_DATA_KEY: "${secrets.dataKey}"${encryption}
`;
}

function buildEnv(secrets: Secrets, encryptionEnabled: boolean) {
  const encryption = encryptionEnabled
    ? `\nTELDRIVE_ENCRYPTION_ACTIVE_KEY_VERSION=1\nTELDRIVE_ENCRYPTION_KEYS=1:${secrets.encryptionKey}`
    : '';

  return `POSTGRES_PASSWORD=${databasePassword}
TELDRIVE_HTTP_ADDRESS=0.0.0.0:8080
TELDRIVE_DATABASE_URL=postgres://teldrive:${databasePassword}@postgres:5432/teldrive?sslmode=disable
TELDRIVE_SECURITY_SIGNING_KEY=${secrets.signingKey}
TELDRIVE_SECURITY_DATA_KEY=${secrets.dataKey}${encryption}
`;
}

function buildYaml(secrets: Secrets, encryptionEnabled: boolean) {
  const encryption = encryptionEnabled
    ? `\nencryption:\n  active-key-version: 1\n  keys:\n    1: "${secrets.encryptionKey}"`
    : '';

  return `http:
  address: 0.0.0.0:8080

database:
  url: "postgres://teldrive:${databasePassword}@127.0.0.1:5432/teldrive?sslmode=disable"

security:
  signing-key: "${secrets.signingKey}"
  data-key: "${secrets.dataKey}"${encryption}
`;
}

function buildCli(secrets: Secrets, encryptionEnabled: boolean) {
  const encryption = encryptionEnabled
    ? ` \\\n  --encryption-active-key-version 1 \\\n  --encryption-keys '1:${secrets.encryptionKey}'`
    : '';

  return `teldrive run \\
  --http-address 0.0.0.0:8080 \\
  --database-url 'postgres://teldrive:${databasePassword}@127.0.0.1:5432/teldrive?sslmode=disable' \\
  --security-signing-key '${secrets.signingKey}' \\
  --security-data-key '${secrets.dataKey}'${encryption}
`;
}

const outputLabels: Record<OutputKind, string> = {
  compose: 'Compose',
  env: '.env',
  yaml: 'YAML',
  cli: 'CLI',
};

function SecretRow({ label, value }: { label: string; value: string }) {
  const copy = async () => {
    await navigator.clipboard.writeText(value);
  };

  return (
    <div className="grid gap-2 border-b border-fd-border py-3 last:border-b-0 sm:grid-cols-[10rem_1fr_auto] sm:items-center">
      <div className="text-sm font-medium text-fd-foreground">{label}</div>
      <code className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap rounded-md bg-fd-secondary px-2 py-1.5 text-xs text-fd-secondary-foreground">
        {value || 'Generating…'}
      </code>
      <button
        type="button"
        onClick={copy}
        disabled={!value}
        className="rounded-md border border-fd-border px-2.5 py-1.5 text-xs font-medium hover:bg-fd-accent disabled:cursor-not-allowed disabled:opacity-50"
      >
        Copy
      </button>
    </div>
  );
}

export default function SetupGenerator() {
  const [secrets, setSecrets] = useState<Secrets | null>(null);
  const [encryptionEnabled, setEncryptionEnabled] = useState(true);
  const [outputKind, setOutputKind] = useState<OutputKind>('compose');
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setSecrets(generateSecrets());
  }, []);

  const output = useMemo(() => {
    if (!secrets) return '';
    switch (outputKind) {
      case 'compose':
        return buildCompose(secrets, encryptionEnabled);
      case 'env':
        return buildEnv(secrets, encryptionEnabled);
      case 'yaml':
        return buildYaml(secrets, encryptionEnabled);
      case 'cli':
        return buildCli(secrets, encryptionEnabled);
    }
  }, [encryptionEnabled, outputKind, secrets]);

  const regenerate = () => {
    setSecrets(generateSecrets());
    setCopied(false);
  };

  const copyOutput = async () => {
    if (!output) return;
    await navigator.clipboard.writeText(output);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="not-prose my-6 overflow-hidden rounded-xl border border-fd-border bg-fd-card text-fd-card-foreground shadow-sm">
      <div className="border-b border-fd-border p-5">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h3 className="m-0 text-lg font-semibold">Teldrive setup generator</h3>
            <p className="mt-1 max-w-2xl text-sm text-fd-muted-foreground">
              Generates Teldrive cryptographic secrets locally in your browser. Teldrive already ships with public Telegram application credentials, so there is nothing to enter here.
            </p>
          </div>
          <button
            type="button"
            onClick={regenerate}
            disabled={!secrets}
            className="shrink-0 rounded-md border border-fd-border px-3 py-2 text-sm font-medium hover:bg-fd-accent disabled:cursor-not-allowed disabled:opacity-50"
          >
            Regenerate keys
          </button>
        </div>
      </div>

      <div className="p-5">
        <div className="rounded-lg border border-fd-border px-4">
          <SecretRow label="Signing key" value={secrets?.signingKey ?? ''} />
          <SecretRow label="Data key" value={secrets?.dataKey ?? ''} />
          {encryptionEnabled ? <SecretRow label="File encryption key" value={secrets?.encryptionKey ?? ''} /> : null}
        </div>

        <label className="mt-4 flex cursor-pointer items-start gap-3 rounded-lg border border-fd-border p-4">
          <input
            type="checkbox"
            checked={encryptionEnabled}
            onChange={(event) => setEncryptionEnabled(event.target.checked)}
            className="mt-1 size-4"
          />
          <span>
            <span className="block text-sm font-medium">Enable server-managed file encryption</span>
            <span className="mt-0.5 block text-xs text-fd-muted-foreground">
              Adds encryption key version 1 and makes it the active key for new encrypted uploads.
            </span>
          </span>
        </label>

        <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
          <div className="inline-flex rounded-lg border border-fd-border p-1">
            <button
              type="button"
              onClick={() => setOutputKind('compose')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${outputKind === 'compose' ? 'bg-fd-accent text-fd-accent-foreground' : 'text-fd-muted-foreground hover:text-fd-foreground'}`}
            >
              Compose
            </button>
            <button
              type="button"
              onClick={() => setOutputKind('env')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${outputKind === 'env' ? 'bg-fd-accent text-fd-accent-foreground' : 'text-fd-muted-foreground hover:text-fd-foreground'}`}
            >
              .env
            </button>
            <button
              type="button"
              onClick={() => setOutputKind('yaml')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${outputKind === 'yaml' ? 'bg-fd-accent text-fd-accent-foreground' : 'text-fd-muted-foreground hover:text-fd-foreground'}`}
            >
              YAML
            </button>
            <button
              type="button"
              onClick={() => setOutputKind('cli')}
              className={`rounded-md px-3 py-1.5 text-sm font-medium ${outputKind === 'cli' ? 'bg-fd-accent text-fd-accent-foreground' : 'text-fd-muted-foreground hover:text-fd-foreground'}`}
            >
              CLI
            </button>
          </div>
          <button
            type="button"
            onClick={copyOutput}
            disabled={!output}
            className="rounded-md bg-fd-primary px-3 py-2 text-sm font-medium text-fd-primary-foreground hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {copied ? 'Copied' : `Copy ${outputLabels[outputKind]}`}
          </button>
        </div>

        <pre className="mt-3 max-h-[32rem] overflow-auto rounded-lg border border-fd-border bg-fd-secondary p-4 text-xs leading-relaxed text-fd-secondary-foreground">
          <code>{output || 'Generating secure values…'}</code>
        </pre>

        <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm">
          <strong>Back up the generated data and encryption keys.</strong> Losing the data key prevents Teldrive from decrypting stored credentials. Losing a file-encryption key makes files encrypted with that key unrecoverable.
        </div>
      </div>
    </div>
  );
}
