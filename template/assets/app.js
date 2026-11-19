(() => {
    const doc = document;
    const root = doc.documentElement;
    const $ = (sel, el = doc) => el.querySelector(sel);
    const $$ = (sel, el = doc) => [...el.querySelectorAll(sel)];
    const calm = matchMedia("(prefers-reduced-motion: reduce)").matches;
    const net = navigator.connection || {};
    const frugal = net.saveData || /2g/.test(net.effectiveType || "");

    // Run a DOM change as a view transition where the browser can.
    const morph = (change) => {
        if (calm || !doc.startViewTransition) {
            change();
            return Promise.resolve();
        }
        return doc.startViewTransition(change).finished.catch(() => {});
    };

    /* ── theme ── */

    const paint = (theme) => {
        root.dataset.theme = theme;
        $('meta[name="theme-color"]').content = theme == "light" ? "#f1efe9" : "#0b0b0c";
    };

    $(".bar-theme").addEventListener("click", (e) => {
        const theme = root.dataset.theme == "light" ? "dark" : "light";
        const box = e.currentTarget.getBoundingClientRect();
        root.style.setProperty("--x", box.left + box.width / 2 + "px");
        root.style.setProperty("--y", box.top + box.height / 2 + "px");
        root.classList.add("theming");
        morph(() => paint(theme)).then(() => root.classList.remove("theming"));
        try {
            localStorage.setItem("theme", theme);
        } catch {}
    });

    // A page restored from the back/forward cache may predate the last switch.
    addEventListener("pageshow", () => {
        try {
            const theme = localStorage.getItem("theme");
            if (theme == "light" || theme == "dark") paint(theme);
        } catch {}
    });

    /* ── album: lightbox ── */

    const shots = $$(".ph");
    if (!shots.length) return;

    const pad = (n) => String(n).padStart(3, "0");
    const thumb = (i) => $("img", shots[i]);
    let lb, stage, frame, small, full, count, orig, info;
    let cur = 0;

    const build = () => {
        lb = doc.createElement("dialog");
        lb.className = "lb";
        lb.setAttribute("aria-label", "照片");
        lb.innerHTML =
            '<div class="lb-top"><span class="lb-count"></span>' +
            '<a class="lb-orig" target="_blank" rel="noopener">原图 ↗</a>' +
            '<button class="lb-close" type="button" aria-label="关闭">✕</button></div>' +
            '<div class="lb-stage" tabindex="-1" autofocus><div class="lb-frame"><img class="lb-small" alt=""><img class="lb-full" alt=""></div>' +
            '<button class="lb-prev" type="button" aria-label="上一张">←</button>' +
            '<button class="lb-next" type="button" aria-label="下一张">→</button></div>' +
            '<div class="lb-info"></div><i class="lb-wait"></i>';
        doc.body.append(lb);
        [stage, frame, small, full, count, orig, info] = [
            ".lb-stage", ".lb-frame", ".lb-small", ".lb-full", ".lb-count", ".lb-orig", ".lb-info",
        ].map((sel) => $(sel, lb));

        $(".lb-close", lb).onclick = close;
        $(".lb-prev", lb).onclick = () => step(-1);
        $(".lb-next", lb).onclick = () => step(1);
        stage.addEventListener("click", (e) => e.target == stage && close());
        lb.addEventListener("cancel", (e) => (e.preventDefault(), close()));
        lb.addEventListener("keydown", (e) => {
            if (e.key == "ArrowRight") step(1);
            if (e.key == "ArrowLeft") step(-1);
        });

        full.addEventListener("load", () => {
            // The original may be rotated differently from its thumbnail.
            frame.style.setProperty("--r", full.naturalWidth / full.naturalHeight);
            full.classList.add("on");
            lb.classList.remove("busy");
        });
        full.addEventListener("error", () => lb.classList.remove("busy"));

        swipe();
        if (window.visualViewport) {
            visualViewport.addEventListener("resize", () => {
                const zoomed = visualViewport.scale > 1.01;
                lb.classList.toggle("zoomed", zoomed);
                if (zoomed && lb.open) sharpen(true);
            });
        }
    };

    // Load the full-size file, unless the thumbnail already has enough
    // pixels for this screen or the connection asks us to hold back.
    const sharpen = (force) => {
        if (full.getAttribute("src")) return;
        const t = thumb(cur);
        const shown = frame.clientWidth * devicePixelRatio;
        const have = t.naturalWidth || t.width;
        if (!force && (frugal || shown <= have * 1.15)) return;
        lb.classList.add("busy");
        full.src = shots[cur].href;
    };

    const show = (i, dir = 0) => {
        cur = (i + shots.length) % shots.length;
        const a = shots[cur];
        const t = thumb(cur);

        full.classList.remove("on");
        full.removeAttribute("src"); // also abandons a download still in flight
        lb.classList.remove("busy");

        frame.style.setProperty("--r", a.style.getPropertyValue("--r"));
        small.src = t.src;
        small.alt = full.alt = t.alt;
        count.textContent = `${pad(cur + 1)} / ${pad(shots.length)}`;
        orig.href = a.href;
        info.replaceChildren(
            ...(a.dataset.info || "").split("|").filter(Boolean).map((text) => {
                const el = doc.createElement("span");
                el.textContent = text;
                return el;
            }),
        );

        if (dir && !calm) {
            frame.animate(
                [{ opacity: 0, transform: `translateX(${dir * 5}%)` }, { opacity: 1, transform: "none" }],
                { duration: 380, easing: "cubic-bezier(.2,.7,.1,1)" },
            );
        }
        history.replaceState(history.state, "", "#" + encodeURIComponent(a.dataset.name));
        sharpen();
        for (const d of [1, -1]) new Image().src = thumb((cur + d + shots.length) % shots.length).src;
    };

    const step = (dir) => show(cur + dir, dir);

    // Opening and closing morph between the thumbnail and the frame: both
    // carry the same view-transition-name, one before and one after.
    const open = (i, instant) => {
        if (!lb) build();
        const t = thumb(i);
        const enter = () => {
            t.style.viewTransitionName = "";
            lb.showModal();
            show(i);
        };
        if (instant) return enter();
        history.pushState({ lb: true }, "");
        t.style.viewTransitionName = "lb";
        morph(enter);
    };

    const leave = () => {
        if (!lb || !lb.open) return;
        const t = thumb(cur);
        t.scrollIntoView({ block: "nearest", behavior: "instant" });
        morph(() => {
            lb.close();
            shots[cur].focus({ preventScroll: true });
            t.style.viewTransitionName = "lb";
        }).then(() => {
            t.style.viewTransitionName = "";
            full.removeAttribute("src");
        });
    };

    // Closing goes through history so the back button closes it too.
    function close() {
        if (history.state && history.state.lb) return history.back();
        history.replaceState(null, "", location.pathname + location.search);
        leave();
    }

    addEventListener("popstate", () => {
        const at = find();
        if (history.state && history.state.lb && at >= 0) {
            if (!lb || !lb.open) open(at, true);
        } else {
            leave();
        }
    });

    // Touch: swipe sideways to move on, pull down to close.
    function swipe() {
        let id, x0, y0, dx, dy, axis;
        const reset = () => {
            id = null;
            frame.style.translate = "";
            frame.style.opacity = "";
        };
        stage.addEventListener("pointerdown", (e) => {
            if (e.pointerType == "mouse" || lb.classList.contains("zoomed") || id != null) return;
            [id, x0, y0, dx, dy, axis] = [e.pointerId, e.clientX, e.clientY, 0, 0, ""];
        });
        stage.addEventListener("pointermove", (e) => {
            if (e.pointerId != id) return;
            dx = e.clientX - x0;
            dy = e.clientY - y0;
            if (!axis && Math.hypot(dx, dy) > 8) axis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
            if (axis == "x") frame.style.translate = dx + "px";
            if (axis == "y" && dy > 0) {
                frame.style.translate = `0 ${dy}px`;
                frame.style.opacity = Math.max(0.3, 1 - dy / 500);
            }
        });
        stage.addEventListener("pointerup", (e) => {
            if (e.pointerId != id) return;
            const [ax, x, y] = [axis, dx, dy];
            reset();
            if (ax == "x" && Math.abs(x) > 48) step(x < 0 ? 1 : -1);
            if (ax == "y" && y > 90) close();
        });
        stage.addEventListener("pointercancel", reset);
    }

    // The address names the photo on show, so a link to it can be shared.
    function find() {
        let name = location.hash.slice(1);
        try {
            name = decodeURIComponent(name);
        } catch {}
        return name ? shots.findIndex((a) => a.dataset.name == name) : -1;
    }

    $(".gallery").addEventListener("click", (e) => {
        const a = e.target.closest(".ph");
        if (!a || e.button || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
        e.preventDefault();
        open(shots.indexOf(a));
    });

    if (find() >= 0) open(find(), true);
})();
