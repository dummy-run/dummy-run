// Drops socket.io events the target's block list names before the page sends them. It runs as a preload
// script, before the page's own scripts, in every document. The engine puts the block list's patterns in place of the placeholder and passes report, a
// BiDi channel, as the only argument. The patterns use "*" for any run of
// characters but "/" and "?" for one, as Go's path.Match does.
//
// socket.io sends an event as an Engine.IO message packet ("4") holding a socket.io EVENT ("2") or
// BINARY_EVENT ("5") packet: 4 2 [attachments "-"] [namespace ","] [ack id] [JSON array]. The first array
// item is the event name. Over a WebSocket each frame is one packet. Over HTTP long-polling (Engine.IO 4) a
// POST body holds packets separated by the record separator \x1e.
(report) => {
  const patterns = __DR_SOCKET_PATTERNS__;
  // segmentMatch matches one segment, with no "/" in it, against a pattern of "*" and "?" wildcards. It keeps only
  // the last star to fall back to, so it never backtracks further than one pass per star.
  const segmentMatch = (p, s) => {
    let pi = 0, si = 0, star = -1, mark = 0;
    while (si < s.length) {
      if (pi < p.length && (p[pi] === '?' || (p[pi] !== '*' && p[pi] === s[si]))) { pi++; si++; }
      else if (pi < p.length && p[pi] === '*') { star = pi++; mark = si; }
      else if (star >= 0) { pi = star + 1; si = ++mark; }
      else return false;
    }
    while (pi < p.length && p[pi] === '*') pi++;
    return pi === p.length;
  };
  // globMatch is Go's path.Match for "*" and "?": neither matches "/", so the name and the pattern must have the
  // same "/"-separated segments.
  const globMatch = (pattern, name) => {
    const ps = pattern.split('/'), ns = name.split('/');
    return ps.length === ns.length && ps.every((p, i) => segmentMatch(p, ns[i]));
  };
  const blocked = (name) => patterns.some((p) => globMatch(p, name));

  const eventName = (packet) => {
    if (typeof packet !== 'string' || packet[0] !== '4' || (packet[1] !== '2' && packet[1] !== '5')) return null;
    let i = 2;
    if (packet[1] === '5') {
      while (i < packet.length && packet[i] >= '0' && packet[i] <= '9') i++;
      if (packet[i] !== '-') return null;
      i++;
    }
    if (packet[i] === '/') {
      const comma = packet.indexOf(',', i);
      if (comma < 0) return null;
      i = comma + 1;
    }
    while (i < packet.length && packet[i] >= '0' && packet[i] <= '9') i++;
    try {
      const args = JSON.parse(packet.slice(i));
      return Array.isArray(args) && typeof args[0] === 'string' ? args[0] : null;
    } catch {
      return null;
    }
  };

  // drop reports and returns true when the packet is an event the list names.
  const drop = (packet) => {
    const name = eventName(packet);
    if (name === null || !blocked(name)) return false;
    try { report(name); } catch {}
    return true;
  };

  // filterPolling returns the body without the blocked packets, or null when it has none. A body left empty is
  // sent as a single Engine.IO noop packet ("6"), so the transport still sees a well-formed request.
  const filterPolling = (body) => {
    if (typeof body !== 'string') return null;
    const packets = body.split('\x1e');
    const kept = packets.filter((p) => !drop(p));
    if (kept.length === packets.length) return null;
    return kept.length ? kept.join('\x1e') : '6';
  };
  const isPolling = (url) => {
    try {
      const u = new URL(url, location.href);
      return u.searchParams.get('transport') === 'polling' && u.searchParams.get('EIO') === '4';
    } catch {
      return false;
    }
  };

  const wsSend = WebSocket.prototype.send;
  WebSocket.prototype.send = function (data) {
    if (drop(data)) return;
    return wsSend.call(this, data);
  };

  const pollURL = new WeakMap();
  const xhrOpen = XMLHttpRequest.prototype.open;
  const xhrSend = XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open = function (method, url, ...rest) {
    pollURL.set(this, String(method).toUpperCase() === 'POST' && isPolling(String(url)));
    return xhrOpen.call(this, method, url, ...rest);
  };
  XMLHttpRequest.prototype.send = function (body) {
    if (pollURL.get(this)) {
      const kept = filterPolling(body);
      if (kept !== null) return xhrSend.call(this, kept);
    }
    return xhrSend.call(this, body);
  };

  const pageFetch = window.fetch;
  window.fetch = function (input, init) {
    const isRequest = input instanceof Request;
    const url = isRequest ? input.url : String(input);
    const method = String((init && init.method) || (isRequest ? input.method : 'GET')).toUpperCase();
    if (method === 'POST' && isPolling(url)) {
      if (init && init.body !== undefined && init.body !== null) {
        const kept = filterPolling(init.body);
        if (kept !== null) return pageFetch.call(this, input, { ...init, body: kept });
      } else if (isRequest) {
        // The body is inside the Request. Read a copy, and send a new Request if anything was dropped.
        return input.clone().text().then((text) => {
          const kept = filterPolling(text);
          if (kept === null) return pageFetch.call(this, input, init);
          return pageFetch.call(this, new Request(input, { ...(init || {}), body: kept }));
        });
      }
    }
    return pageFetch.call(this, input, init);
  };
}
