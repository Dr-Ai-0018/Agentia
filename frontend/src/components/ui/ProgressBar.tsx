type ProgressBarProps = {
  value: number;
  toneClass?: string;
};

export function ProgressBar({ value, toneClass = "quota-good" }: ProgressBarProps) {
  const width = Math.max(0, Math.min(100, value));
  return (
    <div className="progress">
      <div className={`progress__bar ${toneClass}`} style={{ width: `${width}%` }} />
    </div>
  );
}
