import React, { useEffect, useState } from 'react';
import './AuthBackground.scss';

// New tech-style background for auth pages
// - Light and Dark themes supported
// - Purely declarative SVG + CSS animations (respecting reduced-motion)
const NODE_POS = [
  [180, 200], [420, 260], [760, 180], [1100, 240], [1440, 190],
  [280, 720], [640, 660], [980, 720], [1320, 680], [1600, 300]
];

const LINKS = [
  [0,1],[1,2],[2,3],[3,4],[0,5],[5,6],[6,7],[7,8],[8,9],[3,7],[2,6]
];

const AuthBackground = () => {
  const [theme, setTheme] = useState(() => {
    if (typeof window !== 'undefined' && window.matchMedia) {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    return 'light';
  });

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return;
    const mq = window.matchMedia('(prefers-color-scheme: dark)');
    const onChange = (e) => setTheme(e.matches ? 'dark' : 'light');
    if (mq.addEventListener) mq.addEventListener('change', onChange);
    else if (mq.addListener) mq.addListener(onChange);
    return () => {
      if (mq.removeEventListener) mq.removeEventListener('change', onChange);
      else if (mq.removeListener) mq.removeListener(onChange);
    };
  }, []);

  // ref for movement/parallax
  const rootRef = React.useRef(null);

  // pointer-based parallax (respects prefers-reduced-motion)
  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    const el = rootRef.current;
    if (!el) return;
    let rafId = null;
    const onMove = (e) => {
      const rect = el.getBoundingClientRect();
      const dx = (e.clientX - (rect.left + rect.width / 2)) / (rect.width / 2);
      const dy = (e.clientY - (rect.top + rect.height / 2)) / (rect.height / 2);
      const maxX = 18, maxY = 12;
      const x = Math.max(-1, Math.min(1, dx)) * maxX;
      const y = Math.max(-1, Math.min(1, dy)) * maxY;
      if (rafId) cancelAnimationFrame(rafId);
      rafId = requestAnimationFrame(() => {
        el.style.setProperty('--brand-x', `${x}px`);
        el.style.setProperty('--brand-y', `${y}px`);
      });
    };
    const reset = () => {
      if (rafId) cancelAnimationFrame(rafId);
      el.style.setProperty('--brand-x', `0px`);
      el.style.setProperty('--brand-y', `0px`);
    };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseleave', reset);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseleave', reset);
      if (rafId) cancelAnimationFrame(rafId);
    };
  }, []);

  const rootClass = `auth-background tech new ${theme}-theme`;

  return (
    <div ref={rootRef} className={rootClass} aria-hidden>
      <svg className="auth-geo" viewBox="0 0 1920 1080" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg" focusable="false" aria-hidden>
        <defs>
          <linearGradient id="bgGrad" x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor="var(--bg-grad-start)" />
            <stop offset="100%" stopColor="var(--bg-grad-end)" />
          </linearGradient>

          <linearGradient id="linkGrad" x1="0" x2="1">
            <stop offset="0%" stopColor="var(--link-start)" />
            <stop offset="100%" stopColor="var(--link-end)" />
          </linearGradient>

          <filter id="softBlur" x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="6" result="b" />
            <feMerge>
              <feMergeNode in="b" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>

          <mask id="fadeMask">
            <rect x="0" y="0" width="100%" height="100%" fill="white" />
            <rect className="moving-band" x="-35%" y="0" width="70%" height="100%" fill="black" />
          </mask>
        </defs>

        <rect width="100%" height="100%" fill="url(#bgGrad)" />

        {/* moving scanning band (subtle tech feel) */}
        <rect className="scan-band" width="100%" height="100%" fill="url(#linkGrad)" opacity="0.06" mask="url(#fadeMask)" />

        {/* link lines */}
        <g className="links" stroke="url(#linkGrad)" strokeWidth="1.6" strokeLinecap="round" filter="url(#softBlur)">
          {LINKS.map((pair, idx) => (
            <line key={idx} x1={NODE_POS[pair[0]][0]} y1={NODE_POS[pair[0]][1]} x2={NODE_POS[pair[1]][0]} y2={NODE_POS[pair[1]][1]} className={`link l${idx % 4}`} />
          ))}
        </g>

        {/* nodes */}
        <g className="nodes">
          {NODE_POS.map((p, i) => (
            <g key={i} transform={`translate(${p[0]}, ${p[1]})`} className={`node-group ng${i}`}>
              <circle className="node-outer" r="12" fill="var(--node-outer)" opacity="0.12" />
              <circle className="node" r="4" fill="var(--node)" />
            </g>
          ))}
        </g>

        {/* watermark / brand */}
        <g className="brand" opacity="0.06">
          <text x="12%" y="16%" fontFamily="Inter, Arial, sans-serif" fontWeight="700" fontSize="42" fill="var(--brand)">Open Prompts</text>
        </g>
      </svg>

      <div className="auth-bg-overlay" />
    </div>
  );
};

export default AuthBackground;
