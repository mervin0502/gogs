// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	SETTINGS_OPTIONS base.TplName = "org/settings/options"
	SETTINGS_DELETE  base.TplName = "org/settings/delete"
)

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.HTML(200, SETTINGS_OPTIONS)
}

func SettingsPost(ctx *middleware.Context, form auth.UpdateOrgSettingForm) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_OPTIONS)
		return
	}

	org := ctx.Org.Organization

	// Check if organization name has been changed.
	if org.Name != form.OrgUserName {
		isExist, err := models.IsUserExist(form.OrgUserName)
		if err != nil {
			ctx.Handle(500, "IsUserExist", err)
			return
		} else if isExist {
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), SETTINGS_OPTIONS, &form)
			return
		} else if err = models.ChangeUserName(org, form.OrgUserName); err != nil {
			if err == models.ErrUserNameIllegal {
				ctx.Flash.Error(ctx.Tr("form.illegal_username"))
				ctx.Redirect("/org/" + org.LowerName + "/settings")
				return
			} else {
				ctx.Handle(500, "ChangeUserName", err)
			}
			return
		}
		log.Trace("Organization name changed: %s -> %s", org.Name, form.OrgUserName)
		org.Name = form.OrgUserName
	}

	org.FullName = form.OrgFullName
	org.Email = form.Email
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	org.Avatar = base.EncodeMd5(form.Avatar)
	org.AvatarEmail = form.Avatar
	if err := models.UpdateUser(org); err != nil {
		ctx.Handle(500, "UpdateUser", err)
		return
	}
	log.Trace("Organization setting updated: %s", org.Name)
	ctx.Flash.Success(ctx.Tr("org.settings.update_setting_success"))
	ctx.Redirect("/org/" + org.Name + "/settings")
}

func SettingsDelete(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsDelete"] = true

	org := ctx.Org.Organization
	if ctx.Req.Method == "POST" {
		// TODO: validate password.
		if err := models.DeleteOrganization(org); err != nil {
			switch err {
			case models.ErrUserOwnRepos:
				ctx.Flash.Error(ctx.Tr("form.org_still_own_repo"))
				ctx.Redirect("/org/" + org.LowerName + "/settings/delete")
			default:
				ctx.Handle(500, "DeleteOrganization", err)
			}
		} else {
			log.Trace("Organization deleted: %s", ctx.User.Name)
			ctx.Redirect("/")
		}
		return
	}

	ctx.HTML(200, SETTINGS_DELETE)
}
