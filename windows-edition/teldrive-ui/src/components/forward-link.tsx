import { Link, type LinkProps } from "@tanstack/react-router";
import { forwardRef } from "react";

export const ForwardLink = forwardRef<HTMLAnchorElement, LinkProps>((props, ref) => (
  <Link ref={ref} {...props} />
));
