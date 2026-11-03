'use strict';
// A top-layer popover is not clipped or repositioned by a themed dialog's
// transform/overflow. The DOM parent remains the dialog, preserving modality.
window.nawaPopover={mount(node){const parent=document.querySelector('dialog[open]')||document.body;parent.append(node);if(typeof node.showPopover==='function'){node.setAttribute('popover','manual');Object.assign(node.style,{inset:'auto',margin:'0'});try{node.showPopover()}catch{node.removeAttribute('popover')}}return node}};
