import { useEffect, useState } from "react";
import clsx from "clsx";

import { grow } from "@/utils/classes";

interface TelegramIconProps {
  className?: string;
}
export function TelegramIcon({ className }: TelegramIconProps) {
  const [isMounted, setisMounted] = useState(false);
  useEffect(() => setisMounted(true), []);
  return (
    <svg data-mounted={isMounted} viewBox="0 0 240 240" className={clsx(grow, className)}>
      <circle cx="120" cy="120" r="120" className="fill-primary" />
      <path
        d="M81.486,130.178,52.2,120.636s-3.5-1.42-2.373-4.64c.232-.664.7-1.229,2.1-2.2,6.489-4.523,120.106-45.36,120.106-45.36s3.208-1.081,5.1-.362a2.766,2.766,0,0,1,1.885,2.055,9.357,9.357,0,0,1,.254,2.585c-.009.752-.1,1.449-.169,2.542-.692,11.165-21.4,94.493-21.4,94.493s-1.239,4.876-5.678,5.043A8.13,8.13,0,0,1,146.1,172.5c-8.711-7.493-38.819-27.727-45.472-32.177a1.27,1.27,0,0,1-.546-.9c-.093-.469.417-1.05.417-1.05s52.426-46.6,53.821-51.492c.108-.379-.3-.566-.848-.4-3.482,1.281-63.844,39.4-70.506,43.607A3.21,3.21,0,0,1,81.486,130.178Z"
        className="fill-surface"
      />
    </svg>
  );
}
