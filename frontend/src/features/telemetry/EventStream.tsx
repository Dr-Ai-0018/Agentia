import { eventStream } from "../../data/mockConsole";

export function EventStream() {
  return (
    <div className="event-stream">
      {eventStream.map((event) => (
        <div className={`event-row event-row--${event.tone}`} key={`${event.time}-${event.body}`}>
          <time>{event.time}</time>
          <strong>{event.source}</strong>
          <span>{event.body}</span>
        </div>
      ))}
    </div>
  );
}
