const { h, render } = window.preact;
const { useState, useEffect, useCallback, useMemo, useRef } = window.preactHooks;
const html = window.htm.bind(h);

window.preact = { h, render, useState, useEffect, useCallback, useMemo, useRef, html };
