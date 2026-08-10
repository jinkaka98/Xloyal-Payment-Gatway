export function formatCurrency(value: number, currency = "IDR") {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency,
    maximumFractionDigits: 0,
  }).format(value).replace(/\u00a0/g, "");
}

export function formatDate(value: string, includeTime = true) {
  return new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    ...(includeTime ? { hour: "2-digit", minute: "2-digit" } : {}),
  }).format(new Date(value));
}

export function formatRelativeTime(value: string, now = new Date()) {
  const seconds = Math.round((new Date(value).getTime() - now.getTime()) / 1000);
  const abs = Math.abs(seconds);
  const suffix = seconds <= 0 ? "ago" : "from now";
  if (abs < 60) return `${abs} sec ${suffix}`;
  if (abs < 3600) return `${Math.round(abs / 60)} min ${suffix}`;
  if (abs < 86400) return `${Math.round(abs / 3600)} hr ${suffix}`;
  return `${Math.round(abs / 86400)} day ${suffix}`;
}
