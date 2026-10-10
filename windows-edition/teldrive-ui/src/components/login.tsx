import type { AuthMessage } from "@/types";
import { useSearch } from "@tanstack/react-router";
import { Button, Input, Spinner } from "@tw-material/react";
import { AsYouType, type CountryCode, getPhoneCode } from "libphonenumber-js";
import meta from "libphonenumber-js/metadata.min.json";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import toast from "react-hot-toast";
import useWebSocket from "react-use-websocket";

import QrCode from "@/components/qr-code";
import { TelegramIcon } from "@/components/telegram-icon";

import { $api } from "@/utils/api";
import { getCountryCode } from "@/utils/common";
import { spanishError } from "@/utils/locale";
import { PhoneNoPicker } from "./menus/phone-picker";

const getKeys = Object.keys as <T>(object: T) => (keyof T)[];

export const displayNames = new Intl.DisplayNames(["es"], { type: "region" });

function sortISOCodes(countryCodes: CountryCode[]) {
  return [...countryCodes].sort((countryCodeA, countryCodeB) => {
    const countryA = displayNames.of(countryCodeA) as string;
    const countryB = displayNames.of(countryCodeB) as string;

    return countryA.localeCompare(countryB);
  });
}

export const isoCodes = sortISOCodes(getKeys(meta.countries))
  .filter((x) => x !== "TA" && x !== "AC")
  .map((code) => ({
    code,
    country: displayNames.of(code) as string,
    value: `+${getPhoneCode(code)}`,
  }));

export const isoCodeMap = isoCodes.reduce(
  (acc, value) => {
    acc[value.code] = value;
    return acc;
  },
  {} as Record<CountryCode, (typeof isoCodes)[0]>,
);

function getTypedNumber(value: string, defaultCountryCode = "IN") {
  if (value) {
    const phone = new AsYouType(defaultCountryCode as CountryCode);
    phone.input(value);
    return phone
      .getNumber()
      ?.formatInternational()
      .replace(isoCodeMap[defaultCountryCode].value, "");
  }
  return value;
}

export type FormState = {
  otpCodeHash?: string;
  otpCode: string;
  phoneNumber: string;
  phoneCode: CountryCode;
  password?: string;
};

type LoginType = "qr" | "phone";

const getWebSocketUrl = () => {
  const host = window.location.origin;
  const url = new URL(host);
  return `${url.protocol === "http:" ? "ws" : "wss"}://${url.host}/api/auth/ws`;
};

const initialState = {
  loginType: "phone" as LoginType,
  qrCode: "",
  step: 1,
  isLoading: false,
  form: {
    phoneCode: getCountryCode(),
    phoneNumber: "",
  } as FormState,
};

export const Login = memo(() => {
  const { redirect } = useSearch({ from: "/_auth/login" });

  const [state, setState] = useState(initialState);

  const { control, handleSubmit, getValues, setError } = useForm({
    defaultValues: initialState.form,
  });

  const { sendJsonMessage } = useWebSocket<AuthMessage>(
    getWebSocketUrl(),
    {
      onMessage: (event) => {
        const message: AuthMessage = JSON.parse(event.data);
        handleWebSocketMessage(message);
      },
    },
    true,
  );

  const postLogin = $api.useMutation("post", "/auth/login", {});

  const handleWebSocketMessage = (message: AuthMessage) => {
    if (message.message === "success") {
      postLogin.mutateAsync({ body: message.payload as any }).finally(() => {
        window.location.replace(new URL(redirect || "/", window.location.origin));
      });
    } else if (message.payload?.phoneCodeHash) {
      const otpCodeHash = message.payload.phoneCodeHash as string;
      setState((prev) => ({
        ...prev,
        isLoading: false,
        step: 2,
        form: { ...prev.form, otpCodeHash },
      }));
    } else if (message.payload?.token) {
      setState((prev) => ({
        ...prev,
        qrCode: message.payload.token as string,
      }));
    } else if (message.message === "2FA required") {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        step: 3,
      }));
    } else if (message.message === "PHONE_CODE_INVALID") {
      setState((prev) => ({
        ...prev,
        isLoading: false,
      }));
      setError("otpCode", { message: "Código de verificación incorrecto" });
    } else if (message.type === "error") {
      toast.error(spanishError(message.message));
    }
  };
  const firstCall = useRef(false);

  useEffect(() => {
    if (state.loginType === "qr" && !firstCall.current) {
      sendJsonMessage({ authType: state.loginType });
      firstCall.current = true;
    }
  }, [state.loginType]);

  const onSubmit = useCallback(
    ({ phoneNumber, otpCode, password, phoneCode }: FormState) => {
      if (state.step === 1 && state.loginType === "phone") {
        setState((prev) => ({
          ...prev,
          isLoading: true,
          form: { ...prev.form, phoneNumber, phoneCode },
        }));
        sendJsonMessage({
          authType: state.loginType,
          message: "sendcode",
          phoneNo: `+${getPhoneCode(phoneCode)}${phoneNumber}`,
        });
      } else if (state.step === 2 && state.loginType === "phone") {
        setState((prev) => ({
          ...prev,
          isLoading: true,
        }));
        sendJsonMessage({
          authType: state.loginType,
          message: "signin",
          phoneNo: `+${getPhoneCode(phoneCode)}${phoneNumber}`,
          phoneCode: otpCode,
          phoneCodeHash: state.form.otpCodeHash,
        });
      } else if (state.step === 3) {
        setState((prev) => ({
          ...prev,
          isLoading: true,
        }));
        sendJsonMessage({
          authType: "2fa",
          password,
        });
      }
    },
    [state.form.otpCodeHash, state.loginType, state.step, sendJsonMessage],
  );

  const onInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const value = getTypedNumber(e.target.value, getValues("phoneCode"));
    e.target.value = value || "";
  }, []);

  return (
    <div className="m-auto flex rounded-large max-w-md flex-col justify-center items-center bg-surface mt-6 gap-4 px-4 pt-6 pb-20">
      <form
        autoComplete="off"
        className="w-full flex flex-col items-center gap-8"
        onSubmit={handleSubmit(onSubmit)}
      >
        {state.loginType === "phone" ? (
          <>
            <TelegramIcon className="size-40" />
            {state.step === 1 && (
              <Controller
                name="phoneNumber"
                control={control}
                rules={{ required: true }}
                render={({ field, fieldState: { error } }) => (
                  <Input
                    isRequired
                    size="lg"
                    label="Número de teléfono"
                    labelPlacement="outside"
                    variant="bordered"
                    className="max-w-xs"
                    isInvalid={!!error}
                    errorMessage={error?.message}
                    startContent={
                      <Controller
                        name="phoneCode"
                        control={control}
                        render={({ field: innerField }) => <PhoneNoPicker field={innerField} />}
                      />
                    }
                    {...field}
                    onChange={(e) => {
                      onInputChange(e);
                      field.onChange(e);
                    }}
                  />
                )}
              />
            )}
            {state.step === 2 && (
              <Controller
                name="otpCode"
                control={control}
                rules={{ required: true }}
                render={({ field, fieldState: { error } }) => (
                  <Input
                    isRequired
                    size="lg"
                    label="Código de verificación"
                    labelPlacement="outside"
                    className="max-w-xs"
                    variant="bordered"
                    isInvalid={!!error}
                    errorMessage={error?.message}
                    {...field}
                  />
                )}
              />
            )}
          </>
        ) : (
          <div className="min-h-64 grid place-content-center">
            {state.step !== 3 && state.qrCode && <QrCode qrCode={state.qrCode} />}

            {state.step !== 3 && !state.qrCode && <Spinner className="size-10" />}
          </div>
        )}

        {state.step === 3 && (
          <Controller
            name="password"
            control={control}
            rules={{ required: true }}
            render={({ field, fieldState: { error } }) => (
              <Input
                isRequired
                size="lg"
                label="Contraseña de verificación en dos pasos"
                className="max-w-xs"
                variant="bordered"
                isInvalid={!!error}
                errorMessage={error?.message}
                type="password"
                {...field}
              />
            )}
          />
        )}

        <div className="flex flex-col gap-6 w-full items-center mt-4">
          {(state.loginType === "phone" || state.step === 3) && (
            <Button
              type="submit"
              fullWidth
              variant="filledTonal"
              isLoading={state.isLoading}
              className="max-w-xs text-inherit"
            >
              {state.isLoading ? "Espera…" : state.step === 1 ? "Siguiente" : "Iniciar sesión"}
            </Button>
          )}
          {state.step !== 3 && (
            <Button
              onPress={() =>
                setState((prev) => ({
                  ...prev,
                  loginType: prev.loginType === "qr" ? "phone" : "qr",
                }))
              }
              fullWidth
              variant="filledTonal"
              className="max-w-xs text-inherit"
            >
              {state.loginType === "qr" ? "Acceder por teléfono" : "Acceder con QR"}
            </Button>
          )}
        </div>
      </form>
    </div>
  );
});
