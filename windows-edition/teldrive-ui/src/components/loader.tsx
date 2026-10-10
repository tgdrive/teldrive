import React, { useEffect } from "react";
import LoadingBar, { type LoadingBarRef } from "react-top-loading-bar";

export default function Loader() {
  const ref = React.useRef<LoadingBarRef>(null);

  useEffect(() => {
    ref?.current?.continuousStart();
    return () => {
      ref?.current?.complete();
    };
  }, []);

  return <LoadingBar className="!bg-primary" ref={ref} shadow={false} waitingTime={200} />;
}
