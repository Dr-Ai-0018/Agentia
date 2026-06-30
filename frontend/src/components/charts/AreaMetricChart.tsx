import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

type Point = {
  time: string;
  tokens: number;
  sparkBurn: number;
};

export function AreaMetricChart({ data }: { data: Point[] }) {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data} margin={{ top: 12, right: 8, left: -24, bottom: 0 }}>
        <defs>
          <linearGradient id="tokensFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#3b82f6" stopOpacity={0.32} />
            <stop offset="95%" stopColor="#3b82f6" stopOpacity={0} />
          </linearGradient>
          <linearGradient id="sparkFill" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.26} />
            <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid stroke="rgba(255,255,255,.06)" vertical={false} />
        <XAxis dataKey="time" tick={{ fill: "#71717a", fontSize: 11 }} axisLine={false} tickLine={false} />
        <YAxis tick={{ fill: "#71717a", fontSize: 11 }} axisLine={false} tickLine={false} />
        <Tooltip
          contentStyle={{
            background: "rgba(9,9,11,.94)",
            border: "1px solid rgba(255,255,255,.12)",
            borderRadius: 8,
            color: "#f4f4f5",
          }}
        />
        <Area type="monotone" dataKey="tokens" stroke="#3b82f6" fill="url(#tokensFill)" strokeWidth={2} />
        <Area type="monotone" dataKey="sparkBurn" stroke="#f59e0b" fill="url(#sparkFill)" strokeWidth={2} />
      </AreaChart>
    </ResponsiveContainer>
  );
}
