import { Button, Card, Description, Input, Label, Spinner, Tabs, TextField } from "@heroui/react";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { toast } from "sonner";
import PhoneIcon from "~icons/gravity-ui/person";
import QrIcon from "~icons/gravity-ui/qr-code";
import ShieldIcon from "~icons/gravity-ui/shield-check";
import { $api } from "@/api/client";
import { userMessage } from "@/api/errors";
import { getQueryClient } from "@/lib/queryClient";
import { currentUserQueryOptions } from "@/auth/queries";

type Step = "phone" | "code" | "password";
type Flow = {
  flowId: string;
  expiresAt: string;
  passwordRequired?: boolean;
  state?: string;
  qrUrl?: string;
  qrExpiresAt?: string;
};
type CookieSession = { authenticated: true; expiresAt: string };

export const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>) => ({
    redirect:
      typeof search.redirect === "string" && search.redirect.startsWith("/")
        ? search.redirect
        : "/files",
  }),
  component: LoginPage,
});

function isSession(value: unknown): value is CookieSession {
  return Boolean(value && typeof value === "object" && "authenticated" in value);
}

function LoginPage() {
  const navigate = useNavigate();
  const { redirect } = Route.useSearch();
  const [method, setMethod] = useState<"phone" | "qr">("phone");
  const [step, setStep] = useState<Step>("phone");
  const [flowId, setFlowId] = useState("");
  const [phone, setPhone] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [qrUrl, setQrUrl] = useState("");
  const [qrExpiry, setQrExpiry] = useState("");

  const startPhone = $api.useMutation("post", "/v1/auth/telegram/start");
  const verifyCode = $api.useMutation("post", "/v1/auth/cookie/telegram/verify-code");
  const verifyPassword = $api.useMutation("post", "/v1/auth/cookie/telegram/verify-password");
  const startQr = $api.useMutation("post", "/v1/auth/telegram/qr/start");
  const pollQr = $api.useMutation("post", "/v1/auth/cookie/telegram/qr/poll");
  const pending = startPhone.isPending || verifyCode.isPending || verifyPassword.isPending;

  const finish = async () => {
    const query = currentUserQueryOptions();
    const qc = getQueryClient();
    await qc.invalidateQueries({ queryKey: query.queryKey });
    await qc.ensureQueryData(query);
    toast.success("Signed in to Teldrive");
    await navigate({ to: redirect, replace: true });
  };

  const submitPhone = async () => {
    try {
      if (step === "phone") {
        const result = (await startPhone.mutateAsync({
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: { phoneNumber: phone.trim() },
        })) as Flow;
        setFlowId(result.flowId);
        setStep(result.passwordRequired ? "password" : "code");
        return;
      }
      if (step === "code") {
        const result = await verifyCode.mutateAsync({
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: { flowId, code: code.trim() },
        });
        if (isSession(result)) await finish();
        else setStep("password");
        return;
      }
      const result = await verifyPassword.mutateAsync({
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { flowId, password },
      });
      if (isSession(result)) await finish();
    } catch (error) {
      toast.error("Telegram sign-in failed", { description: userMessage(error) });
    }
  };

  useEffect(() => {
    if (method !== "qr") return;
    let active = true;
    let timer = 0;
    void startQr
      .mutateAsync({ params: { header: { "Idempotency-Key": crypto.randomUUID() } } })
      .then((result) => {
        if (!active) return;
        const flow = result as Flow;
        setFlowId(flow.flowId);
        setQrUrl(flow.qrUrl ?? "");
        setQrExpiry(flow.qrExpiresAt ?? flow.expiresAt);
        timer = window.setInterval(async () => {
          try {
            const next = await pollQr.mutateAsync({
              params: { header: { "Idempotency-Key": crypto.randomUUID() } },
              body: { flowId: flow.flowId },
            });
            if (!active) return;
            if (isSession(next)) {
              window.clearInterval(timer);
              await finish();
              return;
            }
            const state = next as Flow;
            if (state.state === "password_required") {
              window.clearInterval(timer);
              setMethod("phone");
              setStep("password");
              return;
            }
            if (state.qrUrl) setQrUrl(state.qrUrl);
            if (state.qrExpiresAt) setQrExpiry(state.qrExpiresAt);
          } catch (error) {
            window.clearInterval(timer);
            toast.error("QR sign-in stopped", { description: userMessage(error) });
          }
        }, 2500);
      })
      .catch((error) =>
        toast.error("Unable to create QR sign-in", { description: userMessage(error) }),
      );
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [method]);

  return (
    <main className="grid min-h-dvh bg-background text-foreground lg:grid-cols-[minmax(0,1.1fr)_minmax(24rem,0.9fr)]">
      <section className="hidden border-r border-border bg-sidebar/70 p-12 lg:flex lg:flex-col lg:justify-between">
        <div className="flex size-11 items-center justify-center rounded-xl bg-accent font-semibold text-accent-foreground">
          TD
        </div>
        <div className="max-w-xl">
          <p className="mb-3 text-xs font-medium uppercase tracking-[0.16em] text-accent">
            Teldrive
          </p>
          <h1 className="text-4xl font-semibold tracking-tight">
            Your Telegram-backed cloud drive.
          </h1>
          <p className="mt-4 max-w-lg text-sm leading-6 text-muted">
            Manage files, uploads, background jobs, channels, bots, sessions, and API access from
            one focused interface.
          </p>
        </div>
      </section>
      <section className="flex items-center justify-center p-4 sm:p-8 lg:p-12">
        <Card className="w-full max-w-md border border-border bg-surface/90 shadow-xl">
          <Card.Header className="block px-6 pt-6">
            <Card.Title>Sign in with Telegram</Card.Title>
            <Card.Description>
              API keys are reserved for rclone and external clients.
            </Card.Description>
          </Card.Header>
          <Card.Content className="space-y-5 px-6 pb-6">
            <Tabs
              selectedKey={method}
              onSelectionChange={(key) => {
                setMethod(key as "phone" | "qr");
                setStep("phone");
              }}
            >
              <Tabs.ListContainer>
                <Tabs.List aria-label="Sign-in method">
                  <Tabs.Tab id="phone">
                    <PhoneIcon className="size-4" /> Phone
                  </Tabs.Tab>
                  <Tabs.Tab id="qr">
                    <QrIcon className="size-4" /> QR code
                  </Tabs.Tab>
                </Tabs.List>
              </Tabs.ListContainer>
              <Tabs.Panel id="phone" className="space-y-4 pt-4">
                {step === "phone" && (
                  <TextField className="grid gap-1">
                    <Label>Telegram phone number</Label>
                    <Input
                      autoFocus
                      placeholder="+12025550123"
                      value={phone}
                      onChange={(event) => setPhone(event.target.value)}
                    />
                    <Description>Use E.164 format including the country code.</Description>
                  </TextField>
                )}
                {step === "code" && (
                  <TextField className="grid gap-1">
                    <Label>Telegram code</Label>
                    <Input
                      autoFocus
                      inputMode="numeric"
                      value={code}
                      onChange={(event) => setCode(event.target.value)}
                    />
                  </TextField>
                )}
                {step === "password" && (
                  <TextField className="grid gap-1">
                    <Label>Two-step verification password</Label>
                    <Input
                      autoFocus
                      type="password"
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                    />
                  </TextField>
                )}
                <Button
                  className="w-full"
                  onPress={submitPhone}
                  isDisabled={
                    pending ||
                    (step === "phone" ? !phone.trim() : step === "code" ? !code.trim() : !password)
                  }
                >
                  {pending ? <Spinner size="sm" /> : <ShieldIcon className="size-4" />}
                  {step === "phone" ? "Send code" : "Verify and sign in"}
                </Button>
                {step !== "phone" && (
                  <Button
                    variant="ghost"
                    className="w-full"
                    onPress={() => {
                      setStep("phone");
                      setFlowId("");
                      setCode("");
                      setPassword("");
                    }}
                  >
                    Start again
                  </Button>
                )}
              </Tabs.Panel>
              <Tabs.Panel id="qr" className="space-y-4 pt-4">
                <div className="grid min-h-72 place-items-center rounded-xl border border-border bg-white p-5 text-black">
                  {qrUrl ? (
                    <QRCodeSVG value={qrUrl} size={220} aria-label="Telegram sign-in QR code" />
                  ) : (
                    <Spinner size="lg" />
                  )}
                </div>
                <div className="text-center">
                  <p className="font-medium">Scan with Telegram</p>
                  <p className="mt-1 text-xs text-muted">
                    Settings → Devices → Link Desktop Device
                  </p>
                  <p className="mt-2 text-xs text-muted">
                    Expires {qrExpiry ? new Date(qrExpiry).toLocaleTimeString() : "soon"}
                  </p>
                </div>
              </Tabs.Panel>
            </Tabs>
          </Card.Content>
        </Card>
      </section>
    </main>
  );
}
