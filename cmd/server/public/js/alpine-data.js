document.addEventListener('alpine:init', () => {
    let mermaidLoadPromise = null;
    let mermaidInitialized = false;

    function loadMermaid() {
        if (window.mermaid) return Promise.resolve(window.mermaid);
        if (!mermaidLoadPromise) {
            mermaidLoadPromise = new Promise((resolve, reject) => {
                const script = document.createElement('script');
                script.src = '/public/js/mermaid.min.js';
                const mermaidIntegrity = document.querySelector('meta[name="mermaid-integrity"]')?.content;
                if (mermaidIntegrity) {
                    script.integrity = mermaidIntegrity;
                }
                script.async = true;
                script.onload = () => resolve(window.mermaid);
                script.onerror = () => reject(new Error('Failed to load Mermaid.'));
                document.head.appendChild(script);
            });
        }
        return mermaidLoadPromise;
    }

    function normalizeMermaidSource(source) {
        let src = (source || '').replace(/\r\n/g, '\n').trim();
        if (!src) return '';

        // Accept pasted escaped newlines (e.g. "flowchart LR\nA --> B").
        if (!src.includes('\n') && src.includes('\\n')) {
            src = src.replace(/\\n/g, '\n');
        }

        // If users pasted a quoted block, remove outer quotes.
        if ((src.startsWith('"') && src.endsWith('"')) || (src.startsWith("'") && src.endsWith("'"))) {
            src = src.slice(1, -1).trim();
        }

        const typePattern = /(?:^|\n)\s*(flowchart|graph|sequenceDiagram|classDiagram|classDiagram-v2|stateDiagram|stateDiagram-v2|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|xychart-beta|block-beta|packet-beta|architecture)\b/;
        const typeMatch = src.match(typePattern);
        if (typeMatch && typeof typeMatch.index === 'number' && typeMatch.index > 0) {
            src = src.slice(typeMatch.index).trimStart();
        }

        if (src.includes('\n')) return src;

        src = src.replace(/^(\s*(?:flowchart|graph)\s+[A-Za-z]{1,3})\s+(.+)$/, (_, header, body) => {
            // Support one-line flowcharts by splitting repeated edge statements.
            const normalizedBody = body
                .trim()
                .replace(/\s+(?=[A-Za-z_][A-Za-z0-9_]*(?:\[[^\]]*]|\([^\)]*\)|\{[^}]*})?\s*(?:-->|---|-.->|==>))/g, '\n');
            return `${header}\n${normalizedBody}`;
        });
        src = src.replace(/^(\s*(?:sequenceDiagram|classDiagram|classDiagram-v2|stateDiagram|stateDiagram-v2|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|xychart-beta|block-beta|packet-beta|architecture))\s+/, '$1\n');
        return src;
    }

    function hasMermaidDiagramType(source) {
        return /(?:^|\n)\s*(flowchart|graph|sequenceDiagram|classDiagram|classDiagram-v2|stateDiagram|stateDiagram-v2|erDiagram|journey|gantt|pie|mindmap|timeline|gitGraph|quadrantChart|xychart-beta|block-beta|packet-beta|architecture)\b/.test(source);
    }

    // Main app layout — sidebar + dark mode with $watch (arrow fns need registered component).
    Alpine.data('appLayout', () => ({
        sidebarOpen: false,
        darkMode: document.documentElement.classList.contains('dark'),
        init() {
            this.$watch('sidebarOpen', v => {
                if (v) this.$nextTick(() => this.$refs.closeBtn && this.$refs.closeBtn.focus());
            });
            this.$watch('darkMode', v => {
                document.documentElement.classList.toggle('dark', v);
                localStorage.setItem('darkMode', v);
            });
        }
    }));

    // Risk scoring panel (wizard step 3 — current risk).
    // initL/initC: initial likelihood/consequence values.
    // highMin: minimum score for red; lowMax: maximum score for green.
    Alpine.data('riskScorer', (initL, initC, highMin, lowMax) => ({
        l: initL,
        c: initC,
        score() { return this.l * this.c; },
        level(s) { return s >= highMin ? 'red' : s > lowMax ? 'yellow' : 'green'; },
        label(s) { return s >= highMin ? 'High' : s > lowMax ? 'Medium' : 'Low'; },
        bgClass(s) { return s >= highMin ? 'bg-red-500' : s > lowMax ? 'bg-amber-400' : 'bg-green-500'; }
    }));

    // Treatment scoring panel (wizard step 4 — target risk).
    Alpine.data('treatmentScorer', (initL, initC, highMin, lowMax) => ({
        tl: initL,
        tc: initC,
        label(s) { return s >= highMin ? 'High' : s > lowMax ? 'Medium' : 'Low'; },
        bgClass(s) { return s >= highMin ? 'bg-red-500' : s > lowMax ? 'bg-amber-400' : 'bg-green-500'; }
    }));

    // Activity completion toggle — $nextTick callback needs registered component.
    Alpine.data('activityComplete', () => ({
        showComplete: false,
        async toggle() {
            this.showComplete = !this.showComplete;
            if (this.showComplete) {
                await this.$nextTick();
                const el = document.getElementById('completed_by');
                if (el) el.focus();
            }
        }
    }));

    Alpine.data('mermaidPreview', () => ({
        source: '',
        error: '',
        rendering: false,
        init() {
            if (this.$refs.src) {
                this.$nextTick(() => this.render());
            }
        },
        async render() {
            if (this.$refs.src) {
                const el = this.$refs.src;
                this.source = 'value' in el ? el.value : el.textContent;
            }
            const normalized = normalizeMermaidSource(this.source);
            if (!normalized.trim()) return;
            if (!hasMermaidDiagramType(normalized)) {
                this.$refs.out.innerHTML = '';
                this.error = 'Missing diagram type. Start with e.g. "flowchart LR" on the first line.';
                return;
            }
            this.error = '';
            this.rendering = true;
            try {
                const mermaid = await loadMermaid();
                if (!mermaidInitialized) {
                    mermaid.initialize({
                        startOnLoad: false,
                        securityLevel: 'sandbox',
                        maxTextSize: 50000,
                        maxEdges: 500,
                        secure: ['securityLevel', 'secure', 'startOnLoad', 'maxTextSize', 'maxEdges', 'fontFamily', 'themeCSS', 'altFontFamily'],
                        theme: 'default'
                    });
                    mermaidInitialized = true;
                }
                const id = 'mmd-' + Math.random().toString(36).slice(2);
                const result = await mermaid.render(id, normalized);
                this.$refs.out.innerHTML = result.svg;
            } catch (e) {
                this.$refs.out.innerHTML = '';
                const raw = e && e.message ? e.message : '';
                if (/parse error/i.test(raw)) {
                    this.error = 'Invalid Mermaid syntax. Use one connection per line, for example: A --> B';
                } else if (/no diagram type detected/i.test(raw)) {
                    this.error = 'Missing diagram type. Start with e.g. "flowchart LR" on the first line.';
                } else if (raw) {
                    this.error = raw;
                } else {
                    this.error = 'Invalid Mermaid diagram.';
                }
            } finally {
                this.rendering = false;
            }
        }
    }));

});
