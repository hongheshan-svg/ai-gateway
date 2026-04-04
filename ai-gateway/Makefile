include $(TOPDIR)/rules.mk

PKG_NAME:=ai-gateway
PKG_VERSION:=0.1.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/zhengshan/openwrt-ai-gateway.git
PKG_SOURCE_VERSION:=HEAD
PKG_MIRROR_HASH:=skip

PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)
PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/zhengshan/openwrt-ai-gateway
GO_PKG_BUILD_PKG:=$(GO_PKG)/cmd/ai-gateway
GO_PKG_LDFLAGS_X:= \
	main.version=$(PKG_VERSION) \
	main.commit=$(PKG_SOURCE_VERSION)

include $(INCLUDE_DIR)/package.mk
include ../../lang/golang/golang-package.mk

define Package/ai-gateway
  SECTION:=net
  CATEGORY:=Network
  SUBMENU:=Web Servers/Proxies
  TITLE:=AI API Identity Gateway
  DEPENDS:=$(GO_ARCH_DEPENDS) +ca-bundle
  URL:=https://github.com/zhengshan/openwrt-ai-gateway
  PKGARCH:=all
endef

define Package/ai-gateway/description
  AI Gateway is a transparent reverse proxy for AI APIs (Anthropic, OpenAI, Gemini).
  It normalizes device identity, environment fingerprints, and telemetry
  to a single canonical profile — giving you control over what
  information leaves your network.

  Features:
  - Transparent DNS-based interception (no client config needed except CA cert)
  - Full identity/telemetry rewriting for Anthropic Claude Code
  - OpenAI and Google Gemini API support
  - Automatic CA certificate generation and management
  - LuCI web management interface (install luci-app-ai-gateway)
endef

define Package/ai-gateway/conffiles
/etc/config/ai-gateway
/etc/ai-gateway/ca/
endef

define Package/ai-gateway/install
	$(call GoPackage/Package/Install/Bin,$(1))

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/ai-gateway.conf $(1)/etc/config/ai-gateway

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/ai-gateway.init $(1)/etc/init.d/ai-gateway

	$(INSTALL_DIR) $(1)/etc/ai-gateway/ca
endef

define Package/ai-gateway/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] || {
	# Generate device identity on first install if not set
	device_id=$$(uci -q get ai-gateway.canonical.device_id)
	if [ -z "$$device_id" ] || [ "$$device_id" = "auto" ]; then
		new_id=$$(ai-gateway -gen-identity | awk '{print $$2}')
		uci set ai-gateway.canonical.device_id="$$new_id"
		uci commit ai-gateway
	fi
}
exit 0
endef

$(eval $(call GoBinPackage,ai-gateway))
$(eval $(call BuildPackage,ai-gateway))
