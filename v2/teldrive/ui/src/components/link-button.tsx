import { Button, type ButtonProps } from "@heroui/react";
import {
  Link,
  type AnyRouter,
  type LinkComponentProps,
  type RegisteredRouter,
} from "@tanstack/react-router";
import type { ReactNode, Ref } from "react";

type LinkButtonProps<
  TRouter extends AnyRouter,
  TFrom extends string,
  TTo extends string | undefined,
  TMaskFrom extends string,
  TMaskTo extends string,
> = LinkComponentProps<"a", TRouter, TFrom, TTo, TMaskFrom, TMaskTo> &
  Omit<
    ButtonProps,
    keyof LinkComponentProps<"a", TRouter, TFrom, TTo, TMaskFrom, TMaskTo> | "render"
  > & {
    children?: ReactNode;
  };

export function LinkButton<
  TRouter extends AnyRouter = RegisteredRouter,
  const TFrom extends string = string,
  const TTo extends string | undefined = undefined,
  const TMaskFrom extends string = TFrom,
  const TMaskTo extends string = "",
>({ children, ...props }: LinkButtonProps<TRouter, TFrom, TTo, TMaskFrom, TMaskTo>) {
  const linkProps = props as LinkComponentProps<"a", TRouter, TFrom, TTo, TMaskFrom, TMaskTo>;

  return (
    <Button
      {...props}
      render={({ ref, ...buttonProps }) => {
        // @ts-expect-error HeroUI types render props for a button; this render target is an anchor.
        return <Link {...linkProps} {...buttonProps} ref={ref as Ref<HTMLAnchorElement>} />;
      }}
    >
      {children}
    </Button>
  );
}
