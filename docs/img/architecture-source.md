Below is the source for architecture.png in mermaid.js syntax.

Can be rendered with any mermaid.js deployment, for example <https://mermaid-js.github.io/mermaid-live-editor/>.

```no-highlight
%%{init:{
    "theme": "neutral",
    "themeVariables": {
        "fontSize": "16px",
        "clusterBkg": "#E9F8FE",
        "clusterBorder": "#3D5873",
        "mainBkg": "#2DD8A3",
        "primaryColor": "#2970FF"
    }
}}%%

graph LR

subgraph Front-end Server
    nginx(nginx) --> vspd(vspd)
    vspd --> dcrd0(dcrd)
end

subgraph voting3 [Voting Server 3]
    dcrwallet3(dcrwallet) --> dcrd3(dcrd)
end

subgraph voting2 [Voting Server 2]
    dcrwallet2(dcrwallet) --> dcrd2(dcrd)
end

subgraph voting1 [Voting Server 1]
    dcrwallet1(dcrwallet) --> dcrd1(dcrd)
end

vspd ----> dcrwallet1 & dcrwallet2 & dcrwallet3
web(Internet<br />Traffic) ---> nginx
```
