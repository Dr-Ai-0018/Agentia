type SparklineProps = {
  values: number[];
  color?: string;
  fill?: string;
  width?: string | number;
  height?: number;
};

export function Sparkline({
  values,
  color = "currentColor",
  fill = "rgba(31, 35, 40, 0.06)",
  width = "100%",
  height = 32,
}: SparklineProps) {
  if (values.length < 2) return null;
  const w = 240;
  const h = 40;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const span = max - min || 1;
  const step = w / (values.length - 1);
  const points = values
    .map((value, index) => {
      const x = index * step;
      const y = h - ((value - min) / span) * h;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");
  const areaPoints = `${points} ${w},${h} 0,${h}`;

  return (
    <svg viewBox={`0 0 ${w} ${h}`} width={width} height={height} preserveAspectRatio="none">
      <polyline points={areaPoints} fill={fill} stroke="none" />
      <polyline points={points} fill="none" stroke={color} strokeWidth="1.5" />
    </svg>
  );
}
