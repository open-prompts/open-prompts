import React from 'react';
import './AuthBackground.scss';

const AuthBackground = () => {
  return (
    <div className="auth-background" aria-hidden>
      {/* layered CSS neon shapes for performance + SVG for depth */}
      <div className="neon-grid" />

      <div className="neon-blob neon-blob--1" />
      <div className="neon-blob neon-blob--2" />
      <div className="neon-blob neon-blob--3" />

      <svg className="bg-svg" viewBox="0 0 1440 900" preserveAspectRatio="xMidYMid slice" xmlns="http://www.w3.org/2000/svg">
        <defs>
          <linearGradient id="g1" x1="0" x2="1">
            <stop offset="0%" stopColor="var(--neon-cyan)" stopOpacity="0.9" />
            <stop offset="50%" stopColor="var(--neon-pink)" stopOpacity="0.85" />
            <stop offset="100%" stopColor="var(--neon-purple)" stopOpacity="0.9" />
          </linearGradient>
        </defs>

        <rect width="100%" height="100%" fill="url(#g1)" opacity="0.06" />

        <g className="svg-lines" opacity="0.6" strokeWidth="1" strokeLinecap="round" fill="none">
          <path d="M0,160 L1440,160" stroke="rgba(255,255,255,0.02)" />
          <path d="M0,320 L1440,320" stroke="rgba(255,255,255,0.02)" />
          <path d="M0,480 L1440,480" stroke="rgba(255,255,255,0.02)" />
        </g>

        <g className="sparkles" fill="white" opacity="0.6">
          <circle cx="120" cy="80" r="1.4" />
          <circle cx="220" cy="40" r="1.6" />
          <circle cx="340" cy="120" r="1.2" />
          <circle cx="760" cy="30" r="1.6" />
          <circle cx="1020" cy="90" r="1.8" />
          <circle cx="1300" cy="70" r="1.2" />
        </g>

      </svg>
    </div>
  );
};

export default AuthBackground;
