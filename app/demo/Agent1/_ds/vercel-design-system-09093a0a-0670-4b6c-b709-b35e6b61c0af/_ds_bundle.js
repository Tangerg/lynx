/* @ds-bundle: {"format":3,"namespace":"VercelDesignSystem_09093a","components":[{"name":"Badge","sourcePath":"components/Badge.jsx"},{"name":"Button","sourcePath":"components/Button.jsx"},{"name":"Input","sourcePath":"components/Input.jsx"}],"sourceHashes":{"components/Badge.jsx":"9d4c51a54fcd","components/Button.jsx":"fcebcf41d57f","components/Input.jsx":"08907ee99ab1","ui_kits/dashboard/components/Chrome.jsx":"557b36a484db","ui_kits/dashboard/components/DeploymentsTable.jsx":"3f291d27fe71","ui_kits/dashboard/components/Inputs.jsx":"637959538c1a","ui_kits/dashboard/components/ProjectGrid.jsx":"6f1a4a5ec911","ui_kits/dashboard/components/SettingsPanel.jsx":"992d0019249f","ui_kits/dashboard/components/data.js":"0f0ccc3d3a98","ui_kits/marketing/components/Buttons.jsx":"9d39ffce52a6","ui_kits/marketing/components/FeatureGrid.jsx":"c06317a27f4a","ui_kits/marketing/components/Footer.jsx":"c58da8d76d77","ui_kits/marketing/components/Hero.jsx":"855ceee73152","ui_kits/marketing/components/LogoStrip.jsx":"95023b6bec79","ui_kits/marketing/components/NavBar.jsx":"a76440172cf7","ui_kits/marketing/components/PricingGrid.jsx":"d7577352edd0","ui_kits/marketing/components/ShowcaseDark.jsx":"94f650974e15","ui_kits/marketing/components/TabPills.jsx":"2a944f17e7a4","ui_kits/marketing/components/TopBanner.jsx":"9a7efb251155"},"inlinedExternals":[],"unexposedExports":[]} */

(() => {

const __ds_ns = (window.VercelDesignSystem_09093a = window.VercelDesignSystem_09093a || {});

const __ds_scope = {};

(__ds_ns.__errors = __ds_ns.__errors || []);

// components/Badge.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Geist Badge — small status pill. Tone maps to the accent scales
 * (100 background + 900 text): gray · blue · amber · red · green · purple.
 */
function Badge({
  children,
  tone = "gray",
  style,
  ...rest
}) {
  const tones = {
    gray: {
      background: "var(--gray-100)",
      color: "var(--gray-900)"
    },
    blue: {
      background: "var(--blue-200)",
      color: "var(--blue-900)"
    },
    amber: {
      background: "var(--amber-200)",
      color: "var(--amber-900)"
    },
    red: {
      background: "var(--red-200)",
      color: "var(--red-900)"
    },
    green: {
      background: "var(--green-200)",
      color: "var(--green-900)"
    },
    purple: {
      background: "var(--purple-200)",
      color: "var(--purple-900)"
    }
  };
  return /*#__PURE__*/React.createElement("span", _extends({
    style: {
      display: "inline-flex",
      alignItems: "center",
      gap: 6,
      height: 24,
      padding: "0 8px",
      borderRadius: "var(--r-full)",
      font: "500 12px/16px var(--font-sans)",
      ...tones[tone],
      ...style
    }
  }, rest), children);
}
Object.assign(__ds_scope, { Badge });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/Badge.jsx", error: String((e && e.message) || e) }); }

// components/Button.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Geist Button — the official in-product control.
 * Solid gray-1000 primary, hairline secondary, ghost tertiary, red-800 error.
 * 6px radius; sizes sm (32) / md (40) / lg (48).
 */
function Button({
  children,
  variant = "primary",
  size = "md",
  disabled = false,
  onClick,
  style,
  ...rest
}) {
  const [hover, setHover] = React.useState(false);
  const sizes = {
    sm: {
      height: 32,
      padding: "0 6px",
      font: "500 14px/20px var(--font-sans)"
    },
    md: {
      height: 40,
      padding: "0 10px",
      font: "500 14px/20px var(--font-sans)"
    },
    lg: {
      height: 48,
      padding: "0 14px",
      font: "500 16px/20px var(--font-sans)"
    }
  };
  const variants = {
    primary: {
      background: hover ? "#000" : "var(--gray-1000)",
      color: "var(--background-100)",
      boxShadow: "none"
    },
    secondary: {
      background: hover ? "var(--gray-100)" : "var(--background-100)",
      color: "var(--gray-1000)",
      boxShadow: "0 0 0 1px var(--gray-alpha-400) inset"
    },
    tertiary: {
      background: hover ? "var(--gray-alpha-200)" : "transparent",
      color: "var(--gray-1000)",
      boxShadow: "none"
    },
    error: {
      background: hover ? "var(--red-900)" : "var(--red-800)",
      color: "#fff",
      boxShadow: "none"
    }
  };
  const disabledStyle = disabled ? {
    background: "var(--gray-100)",
    color: "var(--gray-700)",
    boxShadow: "none",
    cursor: "not-allowed"
  } : {};
  return /*#__PURE__*/React.createElement("button", _extends({
    type: "button",
    disabled: disabled,
    onClick: disabled ? undefined : onClick,
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      gap: 6,
      border: 0,
      borderRadius: "var(--r-sm)",
      cursor: "pointer",
      whiteSpace: "nowrap",
      transition: "background-color 150ms cubic-bezier(0.175,0.885,0.32,1.1), box-shadow 150ms cubic-bezier(0.175,0.885,0.32,1.1)",
      ...sizes[size],
      ...variants[variant],
      ...disabledStyle,
      ...style
    }
  }, rest), children);
}
Object.assign(__ds_scope, { Button });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/Button.jsx", error: String((e && e.message) || e) }); }

// components/Input.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/**
 * Geist Input — text field with the two-layer focus ring.
 * Sizes sm (32) / md (40) / lg (48); 6px radius, hairline border at rest.
 */
function Input({
  size = "md",
  disabled = false,
  style,
  ...rest
}) {
  const [focus, setFocus] = React.useState(false);
  const sizes = {
    sm: {
      height: 32,
      font: "400 14px/20px var(--font-sans)"
    },
    md: {
      height: 40,
      font: "400 14px/20px var(--font-sans)"
    },
    lg: {
      height: 48,
      font: "400 16px/24px var(--font-sans)"
    }
  };
  return /*#__PURE__*/React.createElement("input", _extends({
    disabled: disabled,
    onFocus: () => setFocus(true),
    onBlur: () => setFocus(false),
    style: {
      width: "100%",
      boxSizing: "border-box",
      padding: "0 12px",
      borderRadius: "var(--r-sm)",
      border: focus ? "1px solid transparent" : "1px solid var(--gray-alpha-400)",
      boxShadow: focus ? "0 0 0 2px #fff, 0 0 0 4px var(--blue-700)" : "none",
      background: disabled ? "var(--gray-100)" : "var(--background-100)",
      color: disabled ? "var(--gray-700)" : "var(--gray-1000)",
      outline: "none",
      transition: "box-shadow 150ms cubic-bezier(0.175,0.885,0.32,1.1)",
      ...sizes[size],
      ...style
    }
  }, rest));
}
Object.assign(__ds_scope, { Input });
})(); } catch (e) { __ds_ns.__errors.push({ path: "components/Input.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/Chrome.jsx
try { (() => {
/* eslint-disable no-undef */

const Avatar = ({
  name = 'A',
  size = 24,
  color = '#171717',
  fg = '#fff'
}) => /*#__PURE__*/React.createElement("span", {
  style: {
    width: size,
    height: size,
    borderRadius: '50%',
    background: color,
    color: fg,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    font: '500 12px/1 var(--font-sans)',
    letterSpacing: 0,
    flex: `0 0 ${size}px`
  }
}, name?.[0]?.toUpperCase());
const Sidebar = ({
  active,
  onNav,
  current
}) => {
  const items = [{
    id: 'overview',
    label: 'Overview',
    icon: 'home'
  }, {
    id: 'deployments',
    label: 'Deployments',
    icon: 'rocket'
  }, {
    id: 'analytics',
    label: 'Analytics',
    icon: 'chart'
  }, {
    id: 'logs',
    label: 'Logs',
    icon: 'logs'
  }, {
    id: 'storage',
    label: 'Storage',
    icon: 'db'
  }, {
    id: 'ai',
    label: 'AI',
    icon: 'sparkle'
  }, {
    id: 'observability',
    label: 'Observability',
    icon: 'eye'
  }, {
    id: 'settings',
    label: 'Settings',
    icon: 'gear'
  }];
  const I = ({
    k
  }) => {
    const props = {
      width: 16,
      height: 16,
      viewBox: '0 0 24 24',
      fill: 'none',
      stroke: 'currentColor',
      strokeWidth: 1.5,
      strokeLinecap: 'round',
      strokeLinejoin: 'round'
    };
    switch (k) {
      case 'home':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M3 11.5 12 4l9 7.5"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M5 10v10h14V10"
        }));
      case 'rocket':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M14 4s5-1 6 0-1 6-1 6L9 20l-5-5L14 4z"
        }), /*#__PURE__*/React.createElement("circle", {
          cx: "15",
          cy: "9",
          r: "1.5"
        }));
      case 'chart':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M3 3v18h18"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M7 14l4-4 3 3 5-6"
        }));
      case 'logs':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M4 4h12l4 4v12H4z"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M8 12h8M8 16h6"
        }));
      case 'db':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("ellipse", {
          cx: "12",
          cy: "5",
          rx: "8",
          ry: "3"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"
        }));
      case 'sparkle':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M12 3l1.6 4.4L18 9l-4.4 1.6L12 15l-1.6-4.4L6 9l4.4-1.6L12 3z"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M19 17l.7 1.8L21.5 19.5l-1.8.7L19 22l-.7-1.8L16.5 19.5l1.8-.7L19 17z"
        }));
      case 'eye':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("path", {
          d: "M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"
        }), /*#__PURE__*/React.createElement("circle", {
          cx: "12",
          cy: "12",
          r: "3"
        }));
      case 'gear':
        return /*#__PURE__*/React.createElement("svg", props, /*#__PURE__*/React.createElement("circle", {
          cx: "12",
          cy: "12",
          r: "3"
        }), /*#__PURE__*/React.createElement("path", {
          d: "M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9 1.65 1.65 0 0 0 4.27 7.18l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09A1.65 1.65 0 0 0 15 4.6a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9c0 .67.39 1.27 1 1.51H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
        }));
      default:
        return null;
    }
  };
  return /*#__PURE__*/React.createElement("aside", {
    style: {
      width: 240,
      background: '#fafafa',
      borderRight: '1px solid #ebebeb',
      padding: '16px 12px',
      display: 'flex',
      flexDirection: 'column',
      gap: 4
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      padding: '6px 10px',
      background: '#fff',
      borderRadius: 6,
      boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
      marginBottom: 12
    }
  }, /*#__PURE__*/React.createElement(Avatar, {
    name: "Acme",
    size: 20,
    color: "#006bff"
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: '500 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px',
      flex: 1
    }
  }, "Acme"), /*#__PURE__*/React.createElement("span", {
    style: {
      color: '#888',
      font: '500 11px/16px var(--font-mono)'
    }
  }, "Pro")), items.map(it => {
    const isActive = it.id === active;
    return /*#__PURE__*/React.createElement("button", {
      key: it.id,
      onClick: () => onNav?.(it.id),
      style: {
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        background: isActive ? '#fff' : 'transparent',
        color: isActive ? '#171717' : '#4d4d4d',
        border: 0,
        cursor: 'pointer',
        padding: '8px 10px',
        borderRadius: 6,
        boxShadow: isActive ? '0 0 0 1px rgba(0,0,0,0.08) inset' : 'none',
        font: '500 14px/20px var(--font-sans)',
        letterSpacing: '-0.28px',
        transition: 'background-color .15s, color .15s',
        textAlign: 'left'
      },
      onMouseEnter: e => {
        if (!isActive) e.currentTarget.style.background = '#f5f5f5';
      },
      onMouseLeave: e => {
        if (!isActive) e.currentTarget.style.background = 'transparent';
      }
    }, /*#__PURE__*/React.createElement(I, {
      k: it.icon
    }), /*#__PURE__*/React.createElement("span", {
      style: {
        flex: 1
      }
    }, it.label));
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      flex: 1
    }
  }), /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      padding: '8px 10px',
      borderRadius: 6,
      background: '#fff',
      boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset'
    }
  }, /*#__PURE__*/React.createElement(Avatar, {
    name: "Sara Kim"
  }), /*#__PURE__*/React.createElement("span", {
    style: {
      font: '500 14px/18px var(--font-sans)',
      letterSpacing: '-0.28px',
      flex: 1
    }
  }, "Sara Kim", /*#__PURE__*/React.createElement("div", {
    style: {
      font: '400 12px/16px var(--font-sans)',
      color: '#888',
      letterSpacing: 0
    }
  }, "sara@acme.com"))));
};
const Breadcrumb = ({
  items
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8
  }
}, items.map((it, i) => /*#__PURE__*/React.createElement(React.Fragment, {
  key: i
}, i > 0 && /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#a1a1a1'
  }
}, "/"), /*#__PURE__*/React.createElement("span", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    font: '500 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    color: i === items.length - 1 ? '#171717' : '#4d4d4d'
  }
}, it.avatar && /*#__PURE__*/React.createElement(Avatar, {
  name: it.avatar,
  size: 18,
  color: it.color || '#171717'
}), it.label))));
const TopBar = ({
  crumbs,
  right
}) => /*#__PURE__*/React.createElement("header", {
  style: {
    height: 56,
    borderBottom: '1px solid #ebebeb',
    background: '#fff',
    padding: '0 24px',
    display: 'flex',
    alignItems: 'center',
    gap: 12
  }
}, /*#__PURE__*/React.createElement(Breadcrumb, {
  items: crumbs
}), /*#__PURE__*/React.createElement("div", {
  style: {
    flex: 1
  }
}), right);
Object.assign(window, {
  Sidebar,
  TopBar,
  Breadcrumb,
  Avatar
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/Chrome.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/DeploymentsTable.jsx
try { (() => {
/* eslint-disable no-undef */

const DeploymentRow = ({
  d
}) => /*#__PURE__*/React.createElement("tr", {
  style: {
    borderBottom: '1px solid #ebebeb'
  }
}, /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    alignItems: 'center',
    gap: 10
  }
}, /*#__PURE__*/React.createElement(StatusBadge, {
  status: d.status
}))), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '500 13px/20px var(--font-mono)',
    color: '#171717'
  }
}, d.sha), /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 13px/18px var(--font-sans)',
    color: '#4d4d4d',
    letterSpacing: '-0.28px',
    marginTop: 2
  }
}, d.message)), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top'
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    font: '400 13px/20px var(--font-mono)',
    color: '#4d4d4d'
  }
}, /*#__PURE__*/React.createElement("svg", {
  width: "12",
  height: "12",
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: "2",
  strokeLinecap: "round",
  "aria-hidden": true
}, /*#__PURE__*/React.createElement("circle", {
  cx: "6",
  cy: "6",
  r: "2"
}), /*#__PURE__*/React.createElement("circle", {
  cx: "18",
  cy: "6",
  r: "2"
}), /*#__PURE__*/React.createElement("circle", {
  cx: "18",
  cy: "18",
  r: "2"
}), /*#__PURE__*/React.createElement("path", {
  d: "M6 8v8M18 8a4 4 0 0 1-4 4H10"
})), d.branch)), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top'
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    font: '500 13px/20px var(--font-sans)',
    color: d.env === 'Production' ? '#006bff' : '#4d4d4d'
  }
}, d.env)), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    alignItems: 'center',
    gap: 8
  }
}, /*#__PURE__*/React.createElement(Avatar, {
  name: d.author,
  size: 20,
  color: "#a1a1a1"
}), /*#__PURE__*/React.createElement("span", {
  style: {
    font: '400 13px/20px var(--font-sans)',
    color: '#4d4d4d',
    letterSpacing: '-0.28px'
  }
}, d.author))), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top',
    font: '400 13px/20px var(--font-mono)',
    color: '#4d4d4d'
  }
}, d.duration), /*#__PURE__*/React.createElement("td", {
  style: {
    padding: '14px 16px',
    verticalAlign: 'top',
    font: '400 13px/20px var(--font-sans)',
    color: '#888',
    letterSpacing: '-0.28px',
    textAlign: 'right'
  }
}, d.when));
const DeploymentsTable = ({
  items
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    background: '#fff',
    borderRadius: 8,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    overflow: 'hidden'
  }
}, /*#__PURE__*/React.createElement("table", {
  style: {
    width: '100%',
    borderCollapse: 'collapse'
  }
}, /*#__PURE__*/React.createElement("thead", null, /*#__PURE__*/React.createElement("tr", {
  style: {
    background: '#fafafa'
  }
}, ['Status', 'Commit', 'Branch', 'Environment', 'Created by', 'Duration', 'Created'].map((h, i) => /*#__PURE__*/React.createElement("th", {
  key: h,
  style: {
    textAlign: i === 6 ? 'right' : 'left',
    padding: '10px 16px',
    font: '500 11px/16px var(--font-mono)',
    letterSpacing: '.08em',
    textTransform: 'uppercase',
    color: '#888',
    borderBottom: '1px solid #ebebeb'
  }
}, h)))), /*#__PURE__*/React.createElement("tbody", null, items.length === 0 && /*#__PURE__*/React.createElement("tr", null, /*#__PURE__*/React.createElement("td", {
  colSpan: 7,
  style: {
    padding: 48,
    textAlign: 'center'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '500 16px/24px var(--font-sans)',
    color: '#171717',
    marginBottom: 4
  }
}, "No deployments match this filter."), /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 14px/20px var(--font-sans)',
    color: '#888',
    letterSpacing: '-0.28px'
  }
}, "Try a different status or environment."))), items.map(d => /*#__PURE__*/React.createElement(DeploymentRow, {
  key: d.sha,
  d: d
})))));
Object.assign(window, {
  DeploymentsTable,
  DeploymentRow
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/DeploymentsTable.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/Inputs.jsx
try { (() => {
/* eslint-disable no-undef */

const IconButton = ({
  children,
  onClick,
  title,
  active
}) => /*#__PURE__*/React.createElement("button", {
  onClick: onClick,
  title: title,
  style: {
    width: 28,
    height: 28,
    borderRadius: 6,
    border: 0,
    cursor: 'pointer',
    background: active ? '#fafafa' : 'transparent',
    color: '#4d4d4d',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    boxShadow: active ? '0 0 0 1px rgba(0,0,0,0.08) inset' : 'none',
    transition: 'background-color .15s, color .15s'
  },
  onMouseEnter: e => {
    e.currentTarget.style.background = '#fafafa';
    e.currentTarget.style.color = '#171717';
  },
  onMouseLeave: e => {
    if (!active) {
      e.currentTarget.style.background = 'transparent';
    }
    e.currentTarget.style.color = active ? '#171717' : '#4d4d4d';
  }
}, children);
const Input = ({
  value,
  onChange,
  placeholder,
  icon,
  width = 240
}) => /*#__PURE__*/React.createElement("label", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    background: '#fff',
    borderRadius: 6,
    height: 32,
    padding: '0 10px',
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    width,
    font: '400 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px'
  }
}, icon && /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888',
    display: 'inline-flex'
  }
}, icon), /*#__PURE__*/React.createElement("input", {
  value: value,
  onChange: e => onChange?.(e.target.value),
  placeholder: placeholder,
  style: {
    border: 0,
    outline: 'none',
    flex: 1,
    background: 'transparent',
    font: 'inherit',
    color: '#171717'
  }
}));
const Select = ({
  value,
  onChange,
  options
}) => /*#__PURE__*/React.createElement("select", {
  value: value,
  onChange: e => onChange?.(e.target.value),
  style: {
    height: 32,
    padding: '0 28px 0 10px',
    borderRadius: 6,
    background: '#fff url("data:image/svg+xml;utf8,<svg xmlns=%27http://www.w3.org/2000/svg%27 width=%2710%27 height=%2710%27 viewBox=%270 0 24 24%27 fill=%27none%27 stroke=%27%23888%27 stroke-width=%272%27 stroke-linecap=%27round%27><path d=%27M6 9l6 6 6-6%27/></svg>") no-repeat right 10px center',
    appearance: 'none',
    WebkitAppearance: 'none',
    border: 0,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    color: '#171717',
    font: '400 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    cursor: 'pointer'
  }
}, options.map(o => /*#__PURE__*/React.createElement("option", {
  key: o.value,
  value: o.value
}, o.label)));
const Tabs = ({
  value,
  onChange,
  items
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    gap: 4,
    borderBottom: '1px solid #ebebeb',
    padding: '0 24px'
  }
}, items.map(t => {
  const active = t.value === value;
  return /*#__PURE__*/React.createElement("button", {
    key: t.value,
    onClick: () => onChange(t.value),
    style: {
      background: 'transparent',
      border: 0,
      cursor: 'pointer',
      padding: '12px 10px',
      marginBottom: -1,
      color: active ? '#171717' : '#4d4d4d',
      font: '500 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px',
      borderBottom: active ? '2px solid #171717' : '2px solid transparent',
      transition: 'color .15s'
    }
  }, t.label);
}));
const STATUS_TONE = {
  Ready: {
    bg: '#dfefff',
    fg: '#0059ec',
    dot: '#006bff'
  },
  Building: {
    bg: '#ffefcf',
    fg: '#ab570a',
    dot: '#f5a623'
  },
  Failed: {
    bg: '#f7d4d6',
    fg: '#c50000',
    dot: '#ee0000'
  },
  Cancelled: {
    bg: '#fafafa',
    fg: '#666',
    dot: '#a1a1a1'
  }
};
const StatusBadge = ({
  status
}) => {
  const t = STATUS_TONE[status] || STATUS_TONE.Cancelled;
  return /*#__PURE__*/React.createElement("span", {
    style: {
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      background: t.bg,
      color: t.fg,
      font: '500 12px/16px var(--font-sans)',
      height: 20,
      padding: '0 8px',
      borderRadius: 9999
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      width: 6,
      height: 6,
      borderRadius: '50%',
      background: t.dot
    }
  }), status);
};
Object.assign(window, {
  IconButton,
  Input,
  Select,
  Tabs,
  StatusBadge,
  STATUS_TONE
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/Inputs.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/ProjectGrid.jsx
try { (() => {
/* eslint-disable no-undef */

const FrameworkIcon = ({
  kind
}) => {
  // Tiny inline framework glyphs (placeholder, all monochrome).
  const sz = 14;
  switch (kind) {
    case 'Next.js':
      return /*#__PURE__*/React.createElement("svg", {
        width: sz,
        height: sz,
        viewBox: "0 0 24 24",
        fill: "currentColor",
        "aria-hidden": true
      }, /*#__PURE__*/React.createElement("circle", {
        cx: "12",
        cy: "12",
        r: "11"
      }), /*#__PURE__*/React.createElement("path", {
        d: "M9 7.5v9M14.5 7.5l-7 9",
        stroke: "#fff",
        strokeWidth: "1.6",
        strokeLinecap: "round"
      }));
    case 'Astro':
      return /*#__PURE__*/React.createElement("svg", {
        width: sz,
        height: sz,
        viewBox: "0 0 24 24",
        fill: "currentColor",
        "aria-hidden": true
      }, /*#__PURE__*/React.createElement("path", {
        d: "M9 3l3 14 3-14M5 18h14a3 3 0 0 1-3 3H8a3 3 0 0 1-3-3z"
      }));
    case 'Edge':
      return /*#__PURE__*/React.createElement("svg", {
        width: sz,
        height: sz,
        viewBox: "0 0 24 24",
        fill: "none",
        stroke: "currentColor",
        strokeWidth: "1.6",
        strokeLinecap: "round",
        strokeLinejoin: "round",
        "aria-hidden": true
      }, /*#__PURE__*/React.createElement("path", {
        d: "M5 12h6M5 7h10M5 17h8M14 9l4 3-4 3"
      }));
    default:
      return /*#__PURE__*/React.createElement("svg", {
        width: sz,
        height: sz,
        viewBox: "0 0 24 24",
        fill: "currentColor",
        "aria-hidden": true
      }, /*#__PURE__*/React.createElement("rect", {
        x: "3",
        y: "3",
        width: "18",
        height: "18",
        rx: "3"
      }));
  }
};
const ProjectCard = ({
  p,
  onOpen
}) => /*#__PURE__*/React.createElement("button", {
  onClick: onOpen,
  style: {
    textAlign: 'left',
    cursor: 'pointer',
    background: '#fff',
    color: '#171717',
    borderRadius: 8,
    border: 0,
    padding: 0,
    boxShadow: '0 2px 2px rgba(0,0,0,0.04), 0 8px 8px -8px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.08) inset',
    overflow: 'hidden',
    display: 'flex',
    flexDirection: 'column',
    transition: 'box-shadow .15s'
  },
  onMouseEnter: e => e.currentTarget.style.boxShadow = '0 4px 6px rgba(0,0,0,0.06), 0 12px 16px -8px rgba(0,0,0,0.08), 0 0 0 1px rgba(0,0,0,0.16) inset',
  onMouseLeave: e => e.currentTarget.style.boxShadow = '0 2px 2px rgba(0,0,0,0.04), 0 8px 8px -8px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.08) inset'
}, /*#__PURE__*/React.createElement("div", {
  style: {
    aspectRatio: '16 / 9',
    background: '#f5f5f5',
    borderBottom: '1px solid #ebebeb',
    position: 'relative',
    overflow: 'hidden'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    position: 'absolute',
    inset: 16,
    background: '#fff',
    borderRadius: 4,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.06) inset'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    height: 12,
    background: '#fafafa',
    borderBottom: '1px solid #ebebeb'
  }
}), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    gap: 6,
    padding: 8
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    flex: 1,
    height: 6,
    background: '#ebebeb',
    borderRadius: 2
  }
}), /*#__PURE__*/React.createElement("div", {
  style: {
    width: 24,
    height: 6,
    background: '#ebebeb',
    borderRadius: 2
  }
})), /*#__PURE__*/React.createElement("div", {
  style: {
    padding: '4px 8px'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    height: 10,
    width: '60%',
    background: '#171717',
    borderRadius: 2,
    marginBottom: 6
  }
}), /*#__PURE__*/React.createElement("div", {
  style: {
    height: 6,
    width: '90%',
    background: '#ebebeb',
    borderRadius: 2,
    marginBottom: 4
  }
}), /*#__PURE__*/React.createElement("div", {
  style: {
    height: 6,
    width: '75%',
    background: '#ebebeb',
    borderRadius: 2
  }
})))), /*#__PURE__*/React.createElement("div", {
  style: {
    padding: 16
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    marginBottom: 6
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#171717'
  }
}, /*#__PURE__*/React.createElement(FrameworkIcon, {
  kind: p.framework
})), /*#__PURE__*/React.createElement("span", {
  style: {
    font: '500 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    flex: 1
  }
}, p.name), /*#__PURE__*/React.createElement(StatusBadge, {
  status: p.status
})), /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 13px/18px var(--font-mono)',
    color: '#888'
  }
}, p.domain), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    marginTop: 10,
    font: '400 12px/16px var(--font-sans)',
    color: '#888'
  }
}, /*#__PURE__*/React.createElement(Avatar, {
  name: p.repo.split('/')[0],
  size: 16,
  color: "#444"
}), p.repo, /*#__PURE__*/React.createElement("span", {
  style: {
    marginLeft: 'auto'
  }
}, p.updated))));
const ProjectGrid = ({
  items,
  onOpen
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'grid',
    gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
    gap: 16
  }
}, items.map(p => /*#__PURE__*/React.createElement(ProjectCard, {
  key: p.id,
  p: p,
  onOpen: () => onOpen?.(p)
})));
Object.assign(window, {
  ProjectGrid,
  ProjectCard
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/ProjectGrid.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/SettingsPanel.jsx
try { (() => {
/* eslint-disable no-undef */

const SettingsRow = ({
  label,
  hint,
  children
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'grid',
    gridTemplateColumns: '240px 1fr',
    gap: 32,
    padding: '20px 0',
    borderBottom: '1px solid #ebebeb'
  }
}, /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '500 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    color: '#171717'
  }
}, label), hint && /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 13px/18px var(--font-sans)',
    color: '#888',
    marginTop: 2,
    letterSpacing: '-0.28px'
  }
}, hint)), /*#__PURE__*/React.createElement("div", null, children));
const SettingsPanel = () => {
  const [name, setName] = useState('web-marketing');
  const [framework, setFramework] = useState('next');
  const [root, setRoot] = useState('./');
  return /*#__PURE__*/React.createElement("div", {
    style: {
      background: '#fff',
      borderRadius: 8,
      padding: '8px 24px',
      boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset'
    }
  }, /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Project Name",
    hint: "Used in your URLs and dashboard."
  }, /*#__PURE__*/React.createElement(Input, {
    value: name,
    onChange: setName,
    width: 360
  })), /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Framework Preset",
    hint: "Detected automatically."
  }, /*#__PURE__*/React.createElement(Select, {
    value: framework,
    onChange: setFramework,
    options: [{
      value: 'next',
      label: 'Next.js'
    }, {
      value: 'astro',
      label: 'Astro'
    }, {
      value: 'svelte',
      label: 'SvelteKit'
    }, {
      value: 'nuxt',
      label: 'Nuxt'
    }, {
      value: 'other',
      label: 'Other'
    }]
  })), /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Root Directory",
    hint: "The directory where your source code is located."
  }, /*#__PURE__*/React.createElement(Input, {
    value: root,
    onChange: setRoot,
    width: 360
  })), /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Node.js Version"
  }, /*#__PURE__*/React.createElement(Select, {
    value: "20",
    onChange: () => {},
    options: [{
      value: '22',
      label: '22.x'
    }, {
      value: '20',
      label: '20.x (default)'
    }, {
      value: '18',
      label: '18.x'
    }]
  })), /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Production Branch",
    hint: "Pushes here trigger a production deploy."
  }, /*#__PURE__*/React.createElement(Input, {
    value: "main",
    onChange: () => {},
    width: 240,
    icon: /*#__PURE__*/React.createElement("svg", {
      width: "14",
      height: "14",
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      strokeWidth: "2",
      strokeLinecap: "round",
      "aria-hidden": true
    }, /*#__PURE__*/React.createElement("circle", {
      cx: "6",
      cy: "6",
      r: "2"
    }), /*#__PURE__*/React.createElement("circle", {
      cx: "18",
      cy: "18",
      r: "2"
    }), /*#__PURE__*/React.createElement("path", {
      d: "M6 8v10"
    }))
  })), /*#__PURE__*/React.createElement(SettingsRow, {
    label: "Danger zone",
    hint: "Deleting a project is permanent and cannot be undone."
  }, /*#__PURE__*/React.createElement("button", {
    style: {
      background: '#ee0000',
      color: '#fff',
      border: 0,
      cursor: 'pointer',
      height: 32,
      padding: '0 14px',
      borderRadius: 6,
      font: '500 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px'
    }
  }, "Delete project")));
};
Object.assign(window, {
  SettingsPanel,
  SettingsRow
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/SettingsPanel.jsx", error: String((e && e.message) || e) }); }

// ui_kits/dashboard/components/data.js
try { (() => {
/* eslint-disable no-undef */
// Mock data — small, realistic, no real names.

const PROJECTS = [{
  id: 'web-marketing',
  name: 'web-marketing',
  framework: 'Next.js',
  repo: 'acme/web-marketing',
  updated: '2m ago',
  status: 'Ready',
  domain: 'acme.com'
}, {
  id: 'docs',
  name: 'docs',
  framework: 'Next.js',
  repo: 'acme/docs',
  updated: '1h ago',
  status: 'Ready',
  domain: 'docs.acme.com'
}, {
  id: 'storefront',
  name: 'storefront',
  framework: 'Next.js',
  repo: 'acme/storefront',
  updated: '3h ago',
  status: 'Building',
  domain: 'shop.acme.com'
}, {
  id: 'dashboard-app',
  name: 'dashboard-app',
  framework: 'Next.js',
  repo: 'acme/dashboard',
  updated: '12h ago',
  status: 'Ready',
  domain: 'app.acme.com'
}, {
  id: 'edge-api',
  name: 'edge-api',
  framework: 'Edge',
  repo: 'acme/edge-api',
  updated: '1d ago',
  status: 'Failed',
  domain: 'api.acme.com'
}, {
  id: 'mobile-web',
  name: 'mobile-web',
  framework: 'Astro',
  repo: 'acme/mobile-web',
  updated: '2d ago',
  status: 'Ready',
  domain: 'm.acme.com'
}];
const DEPLOYMENTS = [{
  sha: '7a3f2c1',
  env: 'Production',
  branch: 'main',
  message: 'Update hero copy and ship pricing v2',
  author: 'sara.kim',
  duration: '34s',
  when: '2m ago',
  status: 'Ready'
}, {
  sha: 'b91e840',
  env: 'Preview',
  branch: 'feat/pricing',
  message: 'Tweak active CPU table layout',
  author: 'jordan',
  duration: '28s',
  when: '14m ago',
  status: 'Ready'
}, {
  sha: 'f54a229',
  env: 'Preview',
  branch: 'feat/auth',
  message: 'Add SAML SSO callback',
  author: 'marcus.t',
  duration: '51s',
  when: '1h ago',
  status: 'Building'
}, {
  sha: '2c8d176',
  env: 'Preview',
  branch: 'fix/og-img',
  message: 'Restore og:image for /pricing route',
  author: 'lin',
  duration: '12s',
  when: '3h ago',
  status: 'Failed'
}, {
  sha: '910bbed',
  env: 'Production',
  branch: 'main',
  message: 'Bump Next.js to 15.0.3',
  author: 'sara.kim',
  duration: '46s',
  when: '6h ago',
  status: 'Ready'
}, {
  sha: '4d29b3a',
  env: 'Preview',
  branch: 'feat/v0-embed',
  message: 'Embed v0 prompt CTA in homepage',
  author: 'jordan',
  duration: '22s',
  when: '12h ago',
  status: 'Ready'
}, {
  sha: 'e0117b2',
  env: 'Preview',
  branch: 'chore/deps',
  message: 'Update dependencies (auto)',
  author: 'dependabot',
  duration: '38s',
  when: '1d ago',
  status: 'Ready'
}, {
  sha: 'aa50fc8',
  env: 'Preview',
  branch: 'feat/blog',
  message: 'Refactor blog grid to use grid-auto',
  author: 'lin',
  duration: '19s',
  when: '2d ago',
  status: 'Cancelled'
}];
Object.assign(window, {
  PROJECTS,
  DEPLOYMENTS
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/dashboard/components/data.js", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/Buttons.jsx
try { (() => {
/* eslint-disable no-undef */
// Shared button vocabulary — marketing scale (100 px pill) + nav scale (6 px radius).
// useState is already destructured in index.html.

const BtnPrimary = ({
  children,
  size = 'lg',
  onClick,
  style
}) => {
  const base = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    background: '#171717',
    color: '#fff',
    border: 0,
    cursor: 'pointer',
    borderRadius: 100,
    whiteSpace: 'nowrap',
    transition: 'background-color .15s ease, transform .08s ease'
  };
  const sized = size === 'lg' ? {
    height: 48,
    padding: '0 24px',
    font: '500 16px/24px var(--font-sans)'
  } : {
    height: 32,
    padding: '0 16px',
    font: '500 14px/20px var(--font-sans)'
  };
  return /*#__PURE__*/React.createElement("button", {
    style: {
      ...base,
      ...sized,
      ...style
    },
    onClick: onClick,
    onMouseDown: e => e.currentTarget.style.transform = 'translateY(0.5px)',
    onMouseUp: e => e.currentTarget.style.transform = '',
    onMouseLeave: e => {
      e.currentTarget.style.transform = '';
      e.currentTarget.style.background = '#171717';
    },
    onMouseEnter: e => e.currentTarget.style.background = '#000'
  }, children);
};
const BtnSecondary = ({
  children,
  size = 'lg',
  onClick,
  style
}) => {
  const base = {
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    background: '#fff',
    color: '#171717',
    border: 0,
    cursor: 'pointer',
    borderRadius: 100,
    whiteSpace: 'nowrap',
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    transition: 'background-color .15s ease, transform .08s ease'
  };
  const sized = size === 'lg' ? {
    height: 48,
    padding: '0 24px',
    font: '500 16px/24px var(--font-sans)'
  } : {
    height: 32,
    padding: '0 16px',
    font: '500 14px/20px var(--font-sans)'
  };
  return /*#__PURE__*/React.createElement("button", {
    style: {
      ...base,
      ...sized,
      ...style
    },
    onClick: onClick,
    onMouseDown: e => e.currentTarget.style.transform = 'translateY(0.5px)',
    onMouseUp: e => e.currentTarget.style.transform = '',
    onMouseLeave: e => {
      e.currentTarget.style.transform = '';
      e.currentTarget.style.background = '#fff';
    },
    onMouseEnter: e => e.currentTarget.style.background = '#fafafa'
  }, children);
};
const NavCta = ({
  variant = 'login',
  children,
  onClick
}) => {
  const styles = {
    signup: {
      background: '#171717',
      color: '#fff',
      boxShadow: 'none'
    },
    login: {
      background: '#fff',
      color: '#171717',
      boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset'
    },
    askai: {
      background: '#fff',
      color: '#171717',
      boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset'
    }
  };
  return /*#__PURE__*/React.createElement("button", {
    onClick: onClick,
    style: {
      height: 32,
      padding: '0 12px',
      borderRadius: 6,
      border: 0,
      cursor: 'pointer',
      font: '500 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px',
      display: 'inline-flex',
      alignItems: 'center',
      gap: 6,
      transition: 'background-color .15s ease',
      ...styles[variant]
    }
  }, children);
};
const TabPill = ({
  active,
  children,
  onClick
}) => /*#__PURE__*/React.createElement("button", {
  onClick: onClick,
  style: {
    background: active ? '#171717' : '#fff',
    color: active ? '#fff' : '#171717',
    boxShadow: active ? 'none' : '0 0 0 1px rgba(0,0,0,0.08) inset',
    height: 36,
    padding: '0 16px',
    borderRadius: 64,
    border: 0,
    cursor: 'pointer',
    font: '400 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    transition: 'background-color .15s ease, color .15s ease'
  }
}, children);
Object.assign(window, {
  BtnPrimary,
  BtnSecondary,
  NavCta,
  TabPill
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/Buttons.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/FeatureGrid.jsx
try { (() => {
function _extends() { return _extends = Object.assign ? Object.assign.bind() : function (n) { for (var e = 1; e < arguments.length; e++) { var t = arguments[e]; for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]); } return n; }, _extends.apply(null, arguments); }
/* eslint-disable no-undef */
const FeatureCard = ({
  eyebrow,
  title,
  body,
  accent
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    background: '#fff',
    color: '#171717',
    padding: 24,
    borderRadius: 8,
    boxShadow: '0 2px 2px rgba(0,0,0,0.04), 0 8px 8px -8px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.08) inset',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    minHeight: 240
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    width: 56,
    height: 56,
    borderRadius: 8,
    background: accent,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    marginBottom: 4
  }
}), /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 11px/16px var(--font-mono)',
    letterSpacing: '.08em',
    textTransform: 'uppercase',
    color: '#888'
  }
}, eyebrow), /*#__PURE__*/React.createElement("h3", {
  style: {
    margin: 0,
    font: '600 24px/32px var(--font-sans)',
    letterSpacing: '-0.96px'
  }
}, title), /*#__PURE__*/React.createElement("p", {
  style: {
    margin: 0,
    font: '400 14px/22px var(--font-sans)',
    color: '#4d4d4d'
  }
}, body));
const FEATURES = {
  'AI Apps': [{
    eyebrow: 'AI SDK',
    title: 'Stream tokens to the edge.',
    body: 'Run model calls beside your users with the AI SDK and edge functions in one stack.',
    accent: 'linear-gradient(135deg,#007cf0,#00dfd8)'
  }, {
    eyebrow: 'AI Gateway',
    title: 'Route every model.',
    body: 'One endpoint, every provider — Claude, GPT, Llama, Mistral — with per-request observability.',
    accent: 'linear-gradient(135deg,#7928ca,#ff0080)'
  }, {
    eyebrow: 'v0',
    title: 'Generate UI with AI.',
    body: 'Prompt-to-component generation, fully integrated with your design system.',
    accent: 'linear-gradient(135deg,#ff4d4d,#f9cb28)'
  }],
  'Web Apps': [{
    eyebrow: 'Framework Defined Infra',
    title: 'Zero config, every framework.',
    body: 'Next.js, Nuxt, SvelteKit, Astro — auto-detected, auto-deployed.',
    accent: 'linear-gradient(135deg,#007cf0,#00dfd8)'
  }, {
    eyebrow: 'Preview',
    title: 'A URL for every commit.',
    body: 'Stakeholders click a link and see exactly what shipped on the branch.',
    accent: 'linear-gradient(135deg,#7928ca,#ff0080)'
  }, {
    eyebrow: 'Edge Runtime',
    title: 'Ship close to your users.',
    body: 'Functions execute at the network edge in dozens of regions.',
    accent: 'linear-gradient(135deg,#ff4d4d,#f9cb28)'
  }],
  'Ecommerce': [{
    eyebrow: 'Composable',
    title: 'Stitch any commerce stack.',
    body: 'Shopify, BigCommerce, Saleor — Vercel sits in front of them all.',
    accent: 'linear-gradient(135deg,#007cf0,#00dfd8)'
  }, {
    eyebrow: 'ISR',
    title: 'Cache the catalogue.',
    body: 'Incremental revalidation keeps product pages instant without stale prices.',
    accent: 'linear-gradient(135deg,#7928ca,#ff0080)'
  }, {
    eyebrow: 'Edge Config',
    title: 'A/B test without redeploys.',
    body: 'Flag-driven experiments propagate globally in under 100 ms.',
    accent: 'linear-gradient(135deg,#ff4d4d,#f9cb28)'
  }],
  'Marketing': [{
    eyebrow: 'CMS',
    title: 'Any headless CMS.',
    body: 'Contentful, Sanity, Storyblok, Builder — all first-class.',
    accent: 'linear-gradient(135deg,#007cf0,#00dfd8)'
  }, {
    eyebrow: 'Analytics',
    title: 'Real-user data, no scripts.',
    body: 'First-party analytics with zero perf impact.',
    accent: 'linear-gradient(135deg,#7928ca,#ff0080)'
  }, {
    eyebrow: 'SEO',
    title: 'Score 100, by default.',
    body: 'Image optimisation, font preload, edge cache — built in.',
    accent: 'linear-gradient(135deg,#ff4d4d,#f9cb28)'
  }],
  'Platforms': [{
    eyebrow: 'Multi-tenant',
    title: 'A site for every customer.',
    body: 'Programmatic deploys, isolated builds, on-demand subdomains.',
    accent: 'linear-gradient(135deg,#007cf0,#00dfd8)'
  }, {
    eyebrow: 'API',
    title: 'Spin up a deploy from code.',
    body: 'Full REST + SDK access for white-label platforms.',
    accent: 'linear-gradient(135deg,#7928ca,#ff0080)'
  }, {
    eyebrow: 'Domains',
    title: 'Bring any apex domain.',
    body: 'Custom certs, automatic renewals, zero DNS plumbing.',
    accent: 'linear-gradient(135deg,#ff4d4d,#f9cb28)'
  }]
};
const FeatureGrid = ({
  tab
}) => {
  const items = FEATURES[tab] || FEATURES['AI Apps'];
  return /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1100,
      margin: '40px auto 0',
      padding: '0 24px',
      display: 'grid',
      gridTemplateColumns: 'repeat(3, 1fr)',
      gap: 16
    }
  }, items.map(f => /*#__PURE__*/React.createElement(FeatureCard, _extends({
    key: f.title
  }, f))));
};
Object.assign(window, {
  FeatureGrid,
  FeatureCard,
  FEATURES
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/FeatureGrid.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/Footer.jsx
try { (() => {
/* eslint-disable no-undef */
const FooterCol = ({
  label,
  links
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    flexDirection: 'column',
    gap: 12
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 12px/16px var(--font-mono)',
    letterSpacing: '.08em',
    textTransform: 'uppercase',
    color: '#888'
  }
}, label), links.map(l => /*#__PURE__*/React.createElement("a", {
  key: l,
  href: "#",
  style: {
    font: '400 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    color: '#4d4d4d',
    textDecoration: 'none'
  }
}, l)));
const Footer = () => /*#__PURE__*/React.createElement("footer", {
  style: {
    background: '#fff',
    borderTop: '1px solid #ebebeb',
    padding: '64px 24px 32px'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    maxWidth: 1100,
    margin: '0 auto',
    display: 'grid',
    gridTemplateColumns: '1.4fr repeat(4, 1fr)',
    gap: 32
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    flexDirection: 'column',
    gap: 14
  }
}, /*#__PURE__*/React.createElement(Logo, null), /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 13px/20px var(--font-sans)',
    color: '#888'
  }
}, "\xA9 ", new Date().getFullYear(), " Vercel Inc."), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    gap: 12,
    marginTop: 4,
    color: '#888'
  }
}, /*#__PURE__*/React.createElement("a", {
  href: "#",
  style: {
    color: 'inherit'
  },
  "aria-label": "GitHub"
}, /*#__PURE__*/React.createElement("svg", {
  width: "20",
  height: "20",
  viewBox: "0 0 24 24",
  fill: "currentColor",
  "aria-hidden": true
}, /*#__PURE__*/React.createElement("path", {
  d: "M12 0a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2.2c-3.3.7-4-1.6-4-1.6-.6-1.4-1.4-1.7-1.4-1.7-1.1-.8.1-.8.1-.8 1.3.1 2 1.3 2 1.3 1.1 1.9 3 1.4 3.7 1 .1-.8.4-1.4.8-1.7-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2-.1-.3-.5-1.5.1-3.2 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0c2.3-1.6 3.3-1.2 3.3-1.2.7 1.7.2 2.9.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.4.4.8 1.1.8 2.2v3.3c0 .3.2.7.8.6A12 12 0 0 0 12 0z"
}))), /*#__PURE__*/React.createElement("a", {
  href: "#",
  style: {
    color: 'inherit'
  },
  "aria-label": "X"
}, /*#__PURE__*/React.createElement("svg", {
  width: "20",
  height: "20",
  viewBox: "0 0 24 24",
  fill: "currentColor",
  "aria-hidden": true
}, /*#__PURE__*/React.createElement("path", {
  d: "M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231zm-1.161 17.52h1.833L7.084 4.126H5.117z"
}))))), /*#__PURE__*/React.createElement(FooterCol, {
  label: "Product",
  links: ['AI', 'Enterprise', 'Fluid Compute', 'Next.js', 'Observability', 'Previews', 'Security']
}), /*#__PURE__*/React.createElement(FooterCol, {
  label: "Resources",
  links: ['Docs', 'Guides', 'Templates', 'Blog', 'Changelog', 'Customers', 'Roadmap']
}), /*#__PURE__*/React.createElement(FooterCol, {
  label: "Company",
  links: ['About', 'Careers', 'Contact', 'Partners', 'Newsroom']
}), /*#__PURE__*/React.createElement(FooterCol, {
  label: "Legal",
  links: ['Privacy', 'Terms', 'Cookies', 'DPA', 'Status']
})));
Object.assign(window, {
  Footer
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/Footer.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/Hero.jsx
try { (() => {
/* eslint-disable no-undef */
const MeshGradient = ({
  height = 720
}) => /*#__PURE__*/React.createElement("div", {
  "aria-hidden": true,
  style: {
    position: 'absolute',
    inset: 0,
    height,
    background: `
      radial-gradient(at 14% 18%, #007cf0 0%, transparent 50%),
      radial-gradient(at 78% 8%,  #00dfd8 0%, transparent 50%),
      radial-gradient(at 8% 70%,  #7928ca 0%, transparent 45%),
      radial-gradient(at 70% 78%, #ff0080 0%, transparent 50%),
      radial-gradient(at 48% 55%, #ff4d4d 0%, transparent 55%),
      radial-gradient(at 95% 50%, #f9cb28 0%, transparent 45%),
      #ffffff
    `,
    pointerEvents: 'none'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    position: 'absolute',
    inset: 0,
    background: 'linear-gradient(180deg, rgba(255,255,255,0) 55%, #fafafa 100%)'
  }
}));
const Hero = () => /*#__PURE__*/React.createElement("section", {
  style: {
    position: 'relative',
    background: '#fafafa',
    overflow: 'hidden'
  }
}, /*#__PURE__*/React.createElement(MeshGradient, {
  height: 760
}), /*#__PURE__*/React.createElement("div", {
  style: {
    position: 'relative',
    zIndex: 1,
    maxWidth: 1100,
    margin: '0 auto',
    padding: '96px 24px 128px',
    textAlign: 'center'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8,
    background: 'rgba(255,255,255,0.7)',
    backdropFilter: 'blur(8px)',
    padding: '6px 14px',
    borderRadius: 9999,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    font: '400 13px/20px var(--font-sans)',
    color: '#4d4d4d',
    marginBottom: 28
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    font: '500 11px/16px var(--font-mono)',
    letterSpacing: '.08em',
    textTransform: 'uppercase',
    color: '#171717'
  }
}, "New"), "Fluid compute is now generally available", /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "\u2192")), /*#__PURE__*/React.createElement("h1", {
  style: {
    margin: 0,
    font: '600 72px/72px var(--font-sans)',
    letterSpacing: '-3.6px',
    color: '#171717'
  }
}, "Build and deploy on", /*#__PURE__*/React.createElement("br", null), "the AI Cloud."), /*#__PURE__*/React.createElement("p", {
  style: {
    margin: '24px auto 0',
    maxWidth: 600,
    font: '400 18px/28px var(--font-sans)',
    color: '#4d4d4d'
  }
}, "Vercel provides the developer tools and cloud infrastructure to build, scale, and secure a faster, more personalized web."), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    gap: 12,
    justifyContent: 'center',
    marginTop: 32
  }
}, /*#__PURE__*/React.createElement(BtnPrimary, null, "Start Deploying \u2192"), /*#__PURE__*/React.createElement(BtnSecondary, null, "Get a Demo"))));
Object.assign(window, {
  Hero,
  MeshGradient
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/Hero.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/LogoStrip.jsx
try { (() => {
/* eslint-disable no-undef */
// Customer logos rendered as monochrome wordmarks (placeholder, single-tone).
const LogoStrip = () => {
  const logos = ['Notion', 'Adobe', 'Stripe', 'HashiCorp', 'OpenAI', 'Loom', 'Sonos', 'Eventbrite'];
  return /*#__PURE__*/React.createElement("section", {
    style: {
      background: '#fafafa',
      padding: '40px 24px'
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1100,
      margin: '0 auto',
      textAlign: 'center',
      font: '400 13px/20px var(--font-sans)',
      color: '#888',
      letterSpacing: '-0.28px'
    }
  }, "Trusted by the world's leading product teams"), /*#__PURE__*/React.createElement("div", {
    style: {
      maxWidth: 1100,
      margin: '24px auto 0',
      display: 'flex',
      flexWrap: 'wrap',
      justifyContent: 'center',
      gap: 56
    }
  }, logos.map(l => /*#__PURE__*/React.createElement("span", {
    key: l,
    style: {
      font: '600 22px/24px var(--font-sans)',
      letterSpacing: '-1px',
      color: '#a1a1a1',
      opacity: 0.85
    }
  }, l))));
};
Object.assign(window, {
  LogoStrip
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/LogoStrip.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/NavBar.jsx
try { (() => {
/* eslint-disable no-undef */
const Logo = ({
  size = 22,
  withWordmark = true,
  color = '#171717'
}) => /*#__PURE__*/React.createElement("span", {
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 8,
    color
  }
}, /*#__PURE__*/React.createElement("svg", {
  viewBox: "0 0 76 65",
  fill: "none",
  style: {
    height: size,
    width: 'auto'
  },
  "aria-hidden": true
}, /*#__PURE__*/React.createElement("path", {
  d: "M37.5274 0L75.0548 65H0L37.5274 0Z",
  fill: "currentColor"
})), withWordmark && /*#__PURE__*/React.createElement("span", {
  style: {
    font: '600 22px/24px var(--font-sans)',
    letterSpacing: '-1px'
  }
}, "Vercel"));
const NavLink = ({
  children,
  active
}) => {
  const [hover, setHover] = useState(false);
  return /*#__PURE__*/React.createElement("button", {
    onMouseEnter: () => setHover(true),
    onMouseLeave: () => setHover(false),
    style: {
      background: hover ? '#fafafa' : 'transparent',
      color: hover || active ? '#171717' : '#4d4d4d',
      border: 0,
      cursor: 'pointer',
      font: '400 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px',
      padding: '6px 12px',
      borderRadius: 9999,
      display: 'inline-flex',
      alignItems: 'center',
      gap: 4,
      transition: 'color .15s, background-color .15s'
    }
  }, children, /*#__PURE__*/React.createElement("span", {
    style: {
      fontSize: 10,
      opacity: 0.6
    }
  }, "\u25BE"));
};
const NavBar = () => /*#__PURE__*/React.createElement("header", {
  style: {
    position: 'sticky',
    top: 0,
    zIndex: 10,
    background: 'rgba(255,255,255,0.8)',
    backdropFilter: 'blur(12px)',
    WebkitBackdropFilter: 'blur(12px)',
    borderBottom: '1px solid #ebebeb',
    height: 64
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    maxWidth: 1400,
    margin: '0 auto',
    height: '100%',
    padding: '0 24px',
    display: 'flex',
    alignItems: 'center',
    gap: 24
  }
}, /*#__PURE__*/React.createElement(Logo, null), /*#__PURE__*/React.createElement("nav", {
  style: {
    display: 'flex',
    gap: 2,
    marginLeft: 16,
    flex: 1
  }
}, /*#__PURE__*/React.createElement(NavLink, null, "Products"), /*#__PURE__*/React.createElement(NavLink, null, "Solutions"), /*#__PURE__*/React.createElement(NavLink, null, "Resources"), /*#__PURE__*/React.createElement(NavLink, null, "Enterprise"), /*#__PURE__*/React.createElement(NavLink, null, "Docs"), /*#__PURE__*/React.createElement(NavLink, null, "Pricing")), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    gap: 8,
    alignItems: 'center'
  }
}, /*#__PURE__*/React.createElement("button", {
  style: {
    background: 'transparent',
    color: '#4d4d4d',
    border: 0,
    cursor: 'pointer',
    font: '400 14px/20px var(--font-sans)',
    padding: '6px 8px'
  }
}, "Contact"), /*#__PURE__*/React.createElement(NavCta, {
  variant: "askai"
}, /*#__PURE__*/React.createElement("span", {
  style: {
    font: '500 11px/16px var(--font-mono)',
    color: '#888'
  }
}, "\u2318K"), "Ask AI"), /*#__PURE__*/React.createElement(NavCta, {
  variant: "login"
}, "Log In"), /*#__PURE__*/React.createElement(NavCta, {
  variant: "signup"
}, "Sign Up"))));
Object.assign(window, {
  Logo,
  NavBar
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/NavBar.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/PricingGrid.jsx
try { (() => {
/* eslint-disable no-undef */
const PricingCard = ({
  tier,
  price,
  period,
  blurb,
  features,
  cta,
  featured
}) => {
  const bg = featured ? '#171717' : '#fff';
  const fg = featured ? '#fff' : '#171717';
  const sub = featured ? '#a1a1a1' : '#4d4d4d';
  return /*#__PURE__*/React.createElement("div", {
    style: {
      background: bg,
      color: fg,
      borderRadius: 12,
      padding: 32,
      boxShadow: featured ? '0 8px 32px -4px rgba(0,0,0,0.25), 0 0 0 1px rgba(255,255,255,0.06) inset' : '0 2px 2px rgba(0,0,0,0.04), 0 8px 16px -4px rgba(0,0,0,0.04), 0 0 0 1px rgba(0,0,0,0.08) inset',
      display: 'flex',
      flexDirection: 'column',
      gap: 16,
      minHeight: 480
    }
  }, /*#__PURE__*/React.createElement("div", {
    style: {
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center'
    }
  }, /*#__PURE__*/React.createElement("span", {
    style: {
      font: '600 18px/24px var(--font-sans)',
      letterSpacing: '-0.6px'
    }
  }, tier), featured && /*#__PURE__*/React.createElement("span", {
    style: {
      font: '500 11px/16px var(--font-mono)',
      letterSpacing: '.08em',
      textTransform: 'uppercase',
      background: 'rgba(255,255,255,0.08)',
      color: '#fff',
      padding: '2px 8px',
      borderRadius: 4
    }
  }, "Most popular")), /*#__PURE__*/React.createElement("div", null, /*#__PURE__*/React.createElement("div", {
    style: {
      font: '600 40px/40px var(--font-sans)',
      letterSpacing: '-2px'
    }
  }, price, /*#__PURE__*/React.createElement("span", {
    style: {
      font: '400 14px/20px var(--font-sans)',
      letterSpacing: 0,
      color: sub,
      marginLeft: 6
    }
  }, period)), /*#__PURE__*/React.createElement("p", {
    style: {
      margin: '12px 0 0',
      font: '400 14px/22px var(--font-sans)',
      color: sub
    }
  }, blurb)), featured ? /*#__PURE__*/React.createElement(BtnSecondary, {
    size: "sm"
  }, cta) : /*#__PURE__*/React.createElement(BtnPrimary, {
    size: "sm"
  }, cta), /*#__PURE__*/React.createElement("div", {
    style: {
      height: 1,
      background: featured ? 'rgba(255,255,255,0.08)' : '#ebebeb'
    }
  }), /*#__PURE__*/React.createElement("ul", {
    style: {
      listStyle: 'none',
      margin: 0,
      padding: 0,
      display: 'flex',
      flexDirection: 'column',
      gap: 10
    }
  }, features.map(f => /*#__PURE__*/React.createElement("li", {
    key: f,
    style: {
      display: 'flex',
      alignItems: 'flex-start',
      gap: 10,
      font: '400 14px/20px var(--font-sans)',
      letterSpacing: '-0.28px',
      color: featured ? '#fff' : '#171717'
    }
  }, /*#__PURE__*/React.createElement("svg", {
    width: "16",
    height: "16",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round",
    strokeLinejoin: "round",
    style: {
      marginTop: 2,
      flex: '0 0 16px'
    },
    "aria-hidden": true
  }, /*#__PURE__*/React.createElement("path", {
    d: "M20 6 9 17l-5-5"
  })), f))));
};
const PricingGrid = () => /*#__PURE__*/React.createElement("section", {
  style: {
    background: '#fafafa',
    padding: '96px 24px'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    maxWidth: 1100,
    margin: '0 auto'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 11px/16px var(--font-mono)',
    letterSpacing: '.16em',
    textTransform: 'uppercase',
    color: '#888',
    textAlign: 'center',
    marginBottom: 16
  }
}, "Pricing"), /*#__PURE__*/React.createElement("h2", {
  style: {
    margin: 0,
    textAlign: 'center',
    font: '600 48px/52px var(--font-sans)',
    letterSpacing: '-2.4px',
    color: '#171717'
  }
}, "Active CPU pricing."), /*#__PURE__*/React.createElement("p", {
  style: {
    margin: '16px auto 56px',
    textAlign: 'center',
    maxWidth: 540,
    font: '400 16px/24px var(--font-sans)',
    color: '#4d4d4d'
  }
}, "Pay only for the milliseconds your code actually runs. No idle time, no surprise bills."), /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'grid',
    gridTemplateColumns: 'repeat(3, 1fr)',
    gap: 16
  }
}, /*#__PURE__*/React.createElement(PricingCard, {
  tier: "Hobby",
  price: "$0",
  period: "/ month forever",
  blurb: "For learning, side-projects, and weekend hacks.",
  cta: "Start free",
  features: ['Personal projects', 'Preview deploys', '100 GB bandwidth', 'Community support']
}), /*#__PURE__*/React.createElement(PricingCard, {
  tier: "Pro",
  price: "$20",
  period: "/ user / month",
  featured: true,
  blurb: "For freelancers and small teams shipping production sites.",
  cta: "Get started \u2192",
  features: ['Team collaboration', '1 TB bandwidth', 'Production analytics', 'Email + chat support', 'Password-protected previews', 'Edge Config']
}), /*#__PURE__*/React.createElement(PricingCard, {
  tier: "Enterprise",
  price: "Custom",
  period: "",
  blurb: "For organisations with SSO, compliance, and scale needs.",
  cta: "Contact Sales",
  features: ['SAML SSO + SCIM', 'SLA & 24/7 support', 'Dedicated infra', 'HIPAA / SOC 2', 'Compliance & audit logs']
}))));
Object.assign(window, {
  PricingGrid,
  PricingCard
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/PricingGrid.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/ShowcaseDark.jsx
try { (() => {
/* eslint-disable no-undef */
const CodeMockup = () => /*#__PURE__*/React.createElement("div", {
  style: {
    background: '#0a0a0a',
    color: '#fff',
    borderRadius: 10,
    padding: 0,
    boxShadow: '0 24px 48px -16px rgba(0,0,0,0.5), 0 0 0 1px rgba(255,255,255,0.06) inset',
    overflow: 'hidden',
    maxWidth: 720,
    margin: '0 auto'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '12px 14px',
    borderBottom: '1px solid rgba(255,255,255,0.08)'
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    width: 12,
    height: 12,
    borderRadius: '50%',
    background: '#3a3a3a'
  }
}), /*#__PURE__*/React.createElement("span", {
  style: {
    width: 12,
    height: 12,
    borderRadius: '50%',
    background: '#3a3a3a'
  }
}), /*#__PURE__*/React.createElement("span", {
  style: {
    width: 12,
    height: 12,
    borderRadius: '50%',
    background: '#3a3a3a'
  }
}), /*#__PURE__*/React.createElement("span", {
  style: {
    marginLeft: 12,
    font: '400 12px/16px var(--font-mono)',
    color: '#888'
  }
}, "~/my-app \u2014 vercel deploy")), /*#__PURE__*/React.createElement("pre", {
  style: {
    margin: 0,
    padding: '20px 24px',
    font: '400 13px/22px var(--font-mono)',
    color: '#e5e5e5',
    whiteSpace: 'pre-wrap'
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "$"), " vercel deploy --prod", '\n', /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "Vercel CLI 32.4.1"), '\n', /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#50e3c2'
  }
}, "\u2713"), "  Production: ", /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#fff',
    textDecoration: 'underline'
  }
}, "https://my-app.vercel.app"), "  ", /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "[2s]"), '\n', /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "\u2022"), "  Cached build \xB7 0.4 s upload \xB7 78 functions deployed", '\n', /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "\u2022"), "  Routed to ", /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#fff'
  }
}, "iad1 \xB7 sfo1 \xB7 fra1 \xB7 gru1"), " + 24 more edges", '\n\n', /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#50e3c2'
  }
}, "\u2713"), "  Done in ", /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#fff'
  }
}, "2.3 s")));
const ShowcaseDark = () => /*#__PURE__*/React.createElement("section", {
  style: {
    background: '#171717',
    color: '#fff',
    padding: '96px 24px'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    maxWidth: 1100,
    margin: '0 auto'
  }
}, /*#__PURE__*/React.createElement("div", {
  style: {
    font: '400 11px/16px var(--font-mono)',
    letterSpacing: '.16em',
    textTransform: 'uppercase',
    color: '#888',
    textAlign: 'center',
    marginBottom: 16
  }
}, "Infrastructure"), /*#__PURE__*/React.createElement("h2", {
  style: {
    margin: 0,
    textAlign: 'center',
    font: '600 48px/52px var(--font-sans)',
    letterSpacing: '-2.4px',
    color: '#fff'
  }
}, "A compute model for", /*#__PURE__*/React.createElement("br", null), "all workloads."), /*#__PURE__*/React.createElement("p", {
  style: {
    margin: '20px auto 56px',
    maxWidth: 580,
    textAlign: 'center',
    font: '400 16px/24px var(--font-sans)',
    color: '#a1a1a1'
  }
}, "One push to Git. We build it, route it, cache it, and observe it \u2014 on infrastructure that scales fluidly with your traffic."), /*#__PURE__*/React.createElement(CodeMockup, null)));
Object.assign(window, {
  ShowcaseDark,
  CodeMockup
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/ShowcaseDark.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/TabPills.jsx
try { (() => {
/* eslint-disable no-undef */
const TABS = ['AI Apps', 'Web Apps', 'Ecommerce', 'Marketing', 'Platforms'];
const TabPills = ({
  value,
  onChange
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    justifyContent: 'center',
    gap: 8,
    padding: '0 24px'
  }
}, TABS.map(t => /*#__PURE__*/React.createElement(TabPill, {
  key: t,
  active: t === value,
  onClick: () => onChange(t)
}, t)));
Object.assign(window, {
  TabPills,
  TABS
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/TabPills.jsx", error: String((e && e.message) || e) }); }

// ui_kits/marketing/components/TopBanner.jsx
try { (() => {
/* eslint-disable no-undef */
const TopBanner = ({
  children = "Introducing v0 — generate UI with AI",
  href = '#'
}) => /*#__PURE__*/React.createElement("div", {
  style: {
    display: 'flex',
    justifyContent: 'center',
    background: '#fff',
    borderBottom: '1px solid #ebebeb',
    padding: '10px 24px'
  }
}, /*#__PURE__*/React.createElement("a", {
  href: href,
  style: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 10,
    background: '#fafafa',
    color: '#4d4d4d',
    font: '400 14px/20px var(--font-sans)',
    letterSpacing: '-0.28px',
    padding: '6px 14px',
    borderRadius: 9999,
    boxShadow: '0 0 0 1px rgba(0,0,0,0.08) inset',
    textDecoration: 'none'
  }
}, /*#__PURE__*/React.createElement("span", {
  style: {
    font: '500 11px/16px var(--font-mono)',
    letterSpacing: '.08em',
    textTransform: 'uppercase',
    color: '#171717',
    background: '#ebebeb',
    padding: '2px 6px',
    borderRadius: 4
  }
}, "New"), /*#__PURE__*/React.createElement("span", null, children), /*#__PURE__*/React.createElement("span", {
  style: {
    color: '#888'
  }
}, "\u2192")));
Object.assign(window, {
  TopBanner
});
})(); } catch (e) { __ds_ns.__errors.push({ path: "ui_kits/marketing/components/TopBanner.jsx", error: String((e && e.message) || e) }); }

__ds_ns.Badge = __ds_scope.Badge;

__ds_ns.Button = __ds_scope.Button;

__ds_ns.Input = __ds_scope.Input;

})();
