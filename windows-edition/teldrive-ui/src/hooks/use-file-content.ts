import fetch from "@/utils/fetch-throw";
import { useEffect, useState } from "react";

export default function useFileContent(url: string) {
  const [response, setResponse] = useState("");
  const [validating, setValidating] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    setValidating(true);
    setError("");
    fetch(url, { signal: controller.signal })
      .then((res) => res.text())
      .then(setResponse)
      .catch((e) => { if (!controller.signal.aborted) setError(e.message); })
      .finally(() => { if (!controller.signal.aborted) setValidating(false); });
    return () => controller.abort();
  }, [url]);
  return { response, error, validating };
}
